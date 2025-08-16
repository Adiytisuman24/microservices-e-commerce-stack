package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "strconv"
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/gorilla/mux"
    "github.com/rs/cors"
)

// InventoryItem represents inventory for a product
type InventoryItem struct {
    ProductID     string `json:"product_id"`
    Available     int    `json:"available"`
    Reserved      int    `json:"reserved"`
    TotalStock    int    `json:"total_stock"`
    LastUpdated   int64  `json:"last_updated"`
}

// Reservation represents a stock reservation
type Reservation struct {
    ReservationID string `json:"reservation_id"`
    ProductID     string `json:"product_id"`
    Quantity      int    `json:"quantity"`
    CartID        string `json:"cart_id"`
    CreatedAt     int64  `json:"created_at"`
    ExpiresAt     int64  `json:"expires_at"`
    Status        string `json:"status"` // reserved, committed, expired
}

// ReservationRequest for creating reservations
type ReservationRequest struct {
    ProductID string `json:"product_id"`
    Quantity  int    `json:"quantity"`
    CartID    string `json:"cart_id"`
}

// StockUpdateRequest for updating stock levels
type StockUpdateRequest struct {
    ProductID string `json:"product_id"`
    Quantity  int    `json:"quantity"`
    Operation string `json:"operation"` // add, set
}

// In-memory stores
var (
    inventory    = make(map[string]InventoryItem)
    reservations = make(map[string]Reservation)
    mu           sync.RWMutex
)

// Constants
const (
    ReservationTimeout = 30 * time.Minute // Reservations expire after 30 minutes
)

// Initialize with sample inventory
func initSampleInventory() {
    sampleProducts := []struct {
        ProductID string
        Stock     int
    }{
        {"sku-12345678", 50},
        {"sku-23456789", 25},
        {"sku-34567890", 100},
        {"sku-45678901", 30},
        {"sku-56789012", 75},
    }

    mu.Lock()
    defer mu.Unlock()

    for _, product := range sampleProducts {
        inventory[product.ProductID] = InventoryItem{
            ProductID:   product.ProductID,
            Available:   product.Stock,
            Reserved:    0,
            TotalStock:  product.Stock,
            LastUpdated: time.Now().Unix(),
        }
    }

    log.Printf("Initialized inventory for %d products", len(sampleProducts))
}

// Health check endpoint
func healthHandler(w http.ResponseWriter, r *http.Request) {
    mu.RLock()
    inventoryCount := len(inventory)
    reservationCount := len(reservations)
    mu.RUnlock()

    health := map[string]interface{}{
        "status":            "healthy",
        "service":           "inventory-service",
        "timestamp":         time.Now().Unix(),
        "inventory_items":   inventoryCount,
        "active_reservations": reservationCount,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(health)
}

// Get inventory for a product
func getInventoryHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    productID := vars["productId"]

    mu.RLock()
    item, exists := inventory[productID]
    mu.RUnlock()

    if !exists {
        http.Error(w, "Product not found in inventory", http.StatusNotFound)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(item)
}

// Get all inventory items
func getAllInventoryHandler(w http.ResponseWriter, r *http.Request) {
    mu.RLock()
    defer mu.RUnlock()

    var items []InventoryItem
    for _, item := range inventory {
        items = append(items, item)
    }

    result := map[string]interface{}{
        "inventory": items,
        "total":     len(items),
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}

// Update stock levels
func updateStockHandler(w http.ResponseWriter, r *http.Request) {
    var req StockUpdateRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    if req.ProductID == "" || req.Quantity < 0 {
        http.Error(w, "Product ID and non-negative quantity required", http.StatusBadRequest)
        return
    }

    mu.Lock()
    defer mu.Unlock()

    item, exists := inventory[req.ProductID]
    if !exists {
        // Create new inventory item
        item = InventoryItem{
            ProductID:   req.ProductID,
            Available:   0,
            Reserved:    0,
            TotalStock:  0,
            LastUpdated: time.Now().Unix(),
        }
    }

    switch req.Operation {
    case "add":
        item.Available += req.Quantity
        item.TotalStock += req.Quantity
    case "set":
        // Ensure we don't set below reserved quantity
        if req.Quantity < item.Reserved {
            http.Error(w, "Cannot set stock below reserved quantity", http.StatusBadRequest)
            return
        }
        item.TotalStock = req.Quantity
        item.Available = req.Quantity - item.Reserved
    default:
        http.Error(w, "Operation must be 'add' or 'set'", http.StatusBadRequest)
        return
    }

    item.LastUpdated = time.Now().Unix()
    inventory[req.ProductID] = item

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(item)
}

// Reserve inventory
func reserveInventoryHandler(w http.ResponseWriter, r *http.Request) {
    var req ReservationRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    if req.ProductID == "" || req.Quantity <= 0 || req.CartID == "" {
        http.Error(w, "Product ID, positive quantity, and cart ID required", http.StatusBadRequest)
        return
    }

    mu.Lock()
    defer mu.Unlock()

    item, exists := inventory[req.ProductID]
    if !exists {
        http.Error(w, "Product not found in inventory", http.StatusNotFound)
        return
    }

    // Check if enough stock is available
    if item.Available < req.Quantity {
        response := map[string]interface{}{
            "success": false,
            "message": fmt.Sprintf("Insufficient stock. Available: %d, Requested: %d", item.Available, req.Quantity),
        }
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(response)
        return
    }

    // Create reservation
    reservation := Reservation{
        ReservationID: uuid.New().String(),
        ProductID:     req.ProductID,
        Quantity:      req.Quantity,
        CartID:        req.CartID,
        CreatedAt:     time.Now().Unix(),
        ExpiresAt:     time.Now().Add(ReservationTimeout).Unix(),
        Status:        "reserved",
    }

    reservations[reservation.ReservationID] = reservation

    // Update inventory
    item.Available -= req.Quantity
    item.Reserved += req.Quantity
    item.LastUpdated = time.Now().Unix()
    inventory[req.ProductID] = item

    response := map[string]interface{}{
        "success":        true,
        "reservation_id": reservation.ReservationID,
        "message":        "Stock reserved successfully",
        "expires_at":     reservation.ExpiresAt,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

// Release reservation
func releaseReservationHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    reservationID := vars["reservationId"]

    mu.Lock()
    defer mu.Unlock()

    reservation, exists := reservations[reservationID]
    if !exists {
        http.Error(w, "Reservation not found", http.StatusNotFound)
        return
    }

    if reservation.Status != "reserved" {
        http.Error(w, "Reservation already processed", http.StatusBadRequest)
        return
    }

    // Update inventory
    item := inventory[reservation.ProductID]
    item.Available += reservation.Quantity
    item.Reserved -= reservation.Quantity
    item.LastUpdated = time.Now().Unix()
    inventory[reservation.ProductID] = item

    // Mark reservation as expired
    reservation.Status = "expired"
    reservations[reservationID] = reservation

    response := map[string]interface{}{
        "success": true,
        "message": "Reservation released successfully",
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

// Commit reservation (convert to actual sale)
func commitReservationHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    reservationID := vars["reservationId"]

    mu.Lock()
    defer mu.Unlock()

    reservation, exists := reservations[reservationID]
    if !exists {
        http.Error(w, "Reservation not found", http.StatusNotFound)
        return
    }

    if reservation.Status != "reserved" {
        http.Error(w, "Reservation already processed", http.StatusBadRequest)
        return
    }

    // Update inventory - reduce total stock
    item := inventory[reservation.ProductID]
    item.Reserved -= reservation.Quantity
    item.TotalStock -= reservation.Quantity
    item.LastUpdated = time.Now().Unix()
    inventory[reservation.ProductID] = item

    // Mark reservation as committed
    reservation.Status = "committed"
    reservations[reservationID] = reservation

    response := map[string]interface{}{
        "success": true,
        "message": "Reservation committed successfully",
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

// Get reservations for a cart
func getCartReservationsHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    cartID := vars["cartId"]

    mu.RLock()
    defer mu.RUnlock()

    var cartReservations []Reservation
    for _, reservation := range reservations {
        if reservation.CartID == cartID && reservation.Status == "reserved" {
            cartReservations = append(cartReservations, reservation)
        }
    }

    result := map[string]interface{}{
        "reservations": cartReservations,
        "count":        len(cartReservations),
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}

// Admin endpoint to clear all inventory
func clearInventoryHandler(w http.ResponseWriter, r *http.Request) {
    mu.Lock()
    defer mu.Unlock()

    inventory = make(map[string]InventoryItem)
    reservations = make(map[string]Reservation)

    result := map[string]string{
        "message": "All inventory and reservations cleared",
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}

// Metrics endpoint
func metricsHandler(w http.ResponseWriter, r *http.Request) {
    mu.RLock()
    inventoryCount := len(inventory)
    reservationCount := 0
    expiredReservations := 0
    
    for _, reservation := range reservations {
        if reservation.Status == "reserved" {
            reservationCount++
        } else if reservation.Status == "expired" {
            expiredReservations++
        }
    }
    mu.RUnlock()

    metrics := fmt.Sprintf(`
# HELP inventory_service_products_total Total number of products in inventory
# TYPE inventory_service_products_total counter
inventory_service_products_total %d

# HELP inventory_service_reservations_active_total Total number of active reservations
# TYPE inventory_service_reservations_active_total counter
inventory_service_reservations_active_total %d

# HELP inventory_service_reservations_expired_total Total number of expired reservations
# TYPE inventory_service_reservations_expired_total counter
inventory_service_reservations_expired_total %d
`, inventoryCount, reservationCount, expiredReservations)

    w.Header().Set("Content-Type", "text/plain")
    w.Write([]byte(metrics))
}

// Background task to clean up expired reservations
func cleanupExpiredReservations() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        mu.Lock()
        now := time.Now().Unix()
        expiredCount := 0

        for reservationID, reservation := range reservations {
            if reservation.Status == "reserved" && now > reservation.ExpiresAt {
                // Release the reservation
                item := inventory[reservation.ProductID]
                item.Available += reservation.Quantity
                item.Reserved -= reservation.Quantity
                item.LastUpdated = now
                inventory[reservation.ProductID] = item

                // Mark as expired
                reservation.Status = "expired"
                reservations[reservationID] = reservation
                expiredCount++
            }
        }

        if expiredCount > 0 {
            log.Printf("Expired %d reservations", expiredCount)
        }
        mu.Unlock()
    }
}

func main() {
    // Initialize sample inventory
    initSampleInventory()

    // Start cleanup goroutine
    go cleanupExpiredReservations()

    router := mux.NewRouter()

    // API routes
    api := router.PathPrefix("/api/inventory").Subrouter()
    api.HandleFunc("", getAllInventoryHandler).Methods("GET")
    api.HandleFunc("/{productId}", getInventoryHandler).Methods("GET")
    api.HandleFunc("/stock", updateStockHandler).Methods("POST")
    api.HandleFunc("/reserve", reserveInventoryHandler).Methods("POST")
    api.HandleFunc("/release/{reservationId}", releaseReservationHandler).Methods("DELETE")
    api.HandleFunc("/commit/{reservationId}", commitReservationHandler).Methods("POST")
    api.HandleFunc("/cart/{cartId}/reservations", getCartReservationsHandler).Methods("GET")

    // Admin routes
    router.HandleFunc("/admin/clear", clearInventoryHandler).Methods("DELETE")

    // Utility routes
    router.HandleFunc("/health", healthHandler).Methods("GET")
    router.HandleFunc("/metrics", metricsHandler).Methods("GET")

    // CORS configuration
    c := cors.New(cors.Options{
        AllowedOrigins:   []string{"*"},
        AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
        AllowedHeaders:   []string{"*"},
        AllowCredentials: true,
    })

    handler := c.Handler(router)

    port := "8004"
    log.Printf("Inventory service starting on port %s", port)
    
    if err := http.ListenAndServe(":"+port, handler); err != nil {
        log.Fatal("Server failed to start:", err)
    }
}