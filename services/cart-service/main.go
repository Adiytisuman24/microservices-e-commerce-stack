package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "strconv"
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/gorilla/mux"
    "github.com/rs/cors"
)

// CartItem represents an item in the cart
type CartItem struct {
    ProductID  string `json:"product_id"`
    Quantity   int    `json:"qty"`
    PriceCents int    `json:"price_cents"`
}

// Cart represents a user's shopping cart
type Cart struct {
    CartID    string     `json:"cart_id"`
    UserID    string     `json:"user_id"`
    Items     []CartItem `json:"items"`
    Reserved  bool       `json:"reserved"`
    UpdatedAt int64      `json:"updated_at"`
}

// AddItemRequest for adding items to cart
type AddItemRequest struct {
    ProductID string `json:"product_id"`
    Quantity  int    `json:"qty"`
}

// ReservationRequest for inventory service
type ReservationRequest struct {
    ProductID string `json:"product_id"`
    Quantity  int    `json:"quantity"`
    CartID    string `json:"cart_id"`
}

// ReservationResponse from inventory service
type ReservationResponse struct {
    Success       bool   `json:"success"`
    ReservationID string `json:"reservation_id"`
    Message       string `json:"message"`
}

// In-memory cart store
var (
    carts       = make(map[string]Cart)
    userCarts   = make(map[string]string) // userID -> cartID mapping
    reservations = make(map[string][]string) // cartID -> reservationIDs
    mu          sync.RWMutex
)

// Environment variables
var (
    inventoryServiceURL = os.Getenv("INVENTORY_SERVICE_URL")
)

func init() {
    if inventoryServiceURL == "" {
        inventoryServiceURL = "http://inventory-service:8004"
    }
}

// Helper function to call inventory service
func reserveInventory(productID string, quantity int, cartID string) (*ReservationResponse, error) {
    if inventoryServiceURL == "" {
        return &ReservationResponse{Success: true, ReservationID: "mock-" + uuid.New().String()[:8]}, nil
    }

    reqData := ReservationRequest{
        ProductID: productID,
        Quantity:  quantity,
        CartID:    cartID,
    }

    jsonData, err := json.Marshal(reqData)
    if err != nil {
        return nil, err
    }

    resp, err := http.Post(
        inventoryServiceURL+"/api/inventory/reserve",
        "application/json",
        bytes.NewBuffer(jsonData),
    )
    if err != nil {
        log.Printf("Failed to call inventory service: %v", err)
        return nil, err
    }
    defer resp.Body.Close()

    var reservationResp ReservationResponse
    if err := json.NewDecoder(resp.Body).Decode(&reservationResp); err != nil {
        return nil, err
    }

    return &reservationResp, nil
}

// Helper function to release inventory reservations
func releaseReservations(cartID string) error {
    mu.RLock()
    reservationIDs := reservations[cartID]
    mu.RUnlock()

    for _, reservationID := range reservationIDs {
        // Call inventory service to release reservation
        url := fmt.Sprintf("%s/api/inventory/release/%s", inventoryServiceURL, reservationID)
        req, _ := http.NewRequest("DELETE", url, nil)
        
        client := &http.Client{Timeout: 5 * time.Second}
        _, err := client.Do(req)
        if err != nil {
            log.Printf("Failed to release reservation %s: %v", reservationID, err)
        }
    }

    // Clear reservations for this cart
    mu.Lock()
    delete(reservations, cartID)
    mu.Unlock()

    return nil
}

// Health check endpoint
func healthHandler(w http.ResponseWriter, r *http.Request) {
    mu.RLock()
    cartCount := len(carts)
    mu.RUnlock()

    health := map[string]interface{}{
        "status":     "healthy",
        "service":    "cart-service",
        "timestamp":  time.Now().Unix(),
        "cart_count": cartCount,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(health)
}

// Get or create cart for user
func getCartHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    userID := vars["userId"]

    mu.Lock()
    defer mu.Unlock()

    // Check if user already has a cart
    cartID, exists := userCarts[userID]
    if !exists {
        // Create new cart
        cartID = uuid.New().String()
        cart := Cart{
            CartID:    cartID,
            UserID:    userID,
            Items:     []CartItem{},
            Reserved:  false,
            UpdatedAt: time.Now().Unix(),
        }
        carts[cartID] = cart
        userCarts[userID] = cartID
    }

    cart := carts[cartID]
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(cart)
}

// Add item to cart
func addItemHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    userID := vars["userId"]

    var req AddItemRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    if req.ProductID == "" || req.Quantity <= 0 {
        http.Error(w, "Product ID and positive quantity required", http.StatusBadRequest)
        return
    }

    mu.Lock()
    defer mu.Unlock()

    // Get or create cart
    cartID, exists := userCarts[userID]
    if !exists {
        cartID = uuid.New().String()
        userCarts[userID] = cartID
    }

    cart, exists := carts[cartID]
    if !exists {
        cart = Cart{
            CartID:    cartID,
            UserID:    userID,
            Items:     []CartItem{},
            Reserved:  false,
            UpdatedAt: time.Now().Unix(),
        }
    }

    // Reserve inventory first
    reservationResp, err := reserveInventory(req.ProductID, req.Quantity, cartID)
    if err != nil {
        http.Error(w, "Failed to reserve inventory", http.StatusInternalServerError)
        return
    }

    if !reservationResp.Success {
        http.Error(w, reservationResp.Message, http.StatusBadRequest)
        return
    }

    // Add or update item in cart
    found := false
    for i, item := range cart.Items {
        if item.ProductID == req.ProductID {
            cart.Items[i].Quantity += req.Quantity
            found = true
            break
        }
    }

    if !found {
        cart.Items = append(cart.Items, CartItem{
            ProductID:  req.ProductID,
            Quantity:   req.Quantity,
            PriceCents: 0, // Should be fetched from product service
        })
    }

    cart.Reserved = true
    cart.UpdatedAt = time.Now().Unix()
    carts[cartID] = cart

    // Track reservations
    if reservations[cartID] == nil {
        reservations[cartID] = []string{}
    }
    reservations[cartID] = append(reservations[cartID], reservationResp.ReservationID)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(cart)
}

// Remove item from cart
func removeItemHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    userID := vars["userId"]
    productID := vars["productId"]

    mu.Lock()
    defer mu.Unlock()

    cartID, exists := userCarts[userID]
    if !exists {
        http.Error(w, "Cart not found", http.StatusNotFound)
        return
    }

    cart, exists := carts[cartID]
    if !exists {
        http.Error(w, "Cart not found", http.StatusNotFound)
        return
    }

    // Find and remove item
    for i, item := range cart.Items {
        if item.ProductID == productID {
            cart.Items = append(cart.Items[:i], cart.Items[i+1:]...)
            break
        }
    }

    cart.UpdatedAt = time.Now().Unix()
    carts[cartID] = cart

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(cart)
}

// Update item quantity
func updateItemHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    userID := vars["userId"]
    productID := vars["productId"]

    quantityStr := r.URL.Query().Get("quantity")
    quantity, err := strconv.Atoi(quantityStr)
    if err != nil || quantity < 0 {
        http.Error(w, "Valid quantity required", http.StatusBadRequest)
        return
    }

    mu.Lock()
    defer mu.Unlock()

    cartID, exists := userCarts[userID]
    if !exists {
        http.Error(w, "Cart not found", http.StatusNotFound)
        return
    }

    cart, exists := carts[cartID]
    if !exists {
        http.Error(w, "Cart not found", http.StatusNotFound)
        return
    }

    // Find and update item
    found := false
    for i, item := range cart.Items {
        if item.ProductID == productID {
            if quantity == 0 {
                // Remove item
                cart.Items = append(cart.Items[:i], cart.Items[i+1:]...)
            } else {
                cart.Items[i].Quantity = quantity
            }
            found = true
            break
        }
    }

    if !found {
        http.Error(w, "Item not found in cart", http.StatusNotFound)
        return
    }

    cart.UpdatedAt = time.Now().Unix()
    carts[cartID] = cart

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(cart)
}

// Clear cart
func clearCartHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    userID := vars["userId"]

    mu.Lock()
    defer mu.Unlock()

    cartID, exists := userCarts[userID]
    if !exists {
        http.Error(w, "Cart not found", http.StatusNotFound)
        return
    }

    // Release all reservations
    go releaseReservations(cartID)

    // Clear cart
    cart := Cart{
        CartID:    cartID,
        UserID:    userID,
        Items:     []CartItem{},
        Reserved:  false,
        UpdatedAt: time.Now().Unix(),
    }
    
    carts[cartID] = cart

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(cart)
}

// Admin endpoint to clear all carts
func clearAllCartsHandler(w http.ResponseWriter, r *http.Request) {
    mu.Lock()
    defer mu.Unlock()

    // Release all reservations
    for cartID := range reservations {
        go releaseReservations(cartID)
    }

    carts = make(map[string]Cart)
    userCarts = make(map[string]string)
    reservations = make(map[string][]string)

    result := map[string]string{
        "message": "All carts cleared",
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}

// Metrics endpoint
func metricsHandler(w http.ResponseWriter, r *http.Request) {
    mu.RLock()
    cartCount := len(carts)
    reservationCount := len(reservations)
    mu.RUnlock()

    metrics := fmt.Sprintf(`
# HELP cart_service_carts_total Total number of carts
# TYPE cart_service_carts_total counter
cart_service_carts_total %d

# HELP cart_service_reservations_total Total number of reservations
# TYPE cart_service_reservations_total counter
cart_service_reservations_total %d
`, cartCount, reservationCount)

    w.Header().Set("Content-Type", "text/plain")
    w.Write([]byte(metrics))
}

// Clean up expired reservations (runs every 30 minutes)
func cleanupExpiredReservations() {
    ticker := time.NewTicker(30 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        mu.Lock()
        now := time.Now().Unix()
        
        for cartID, cart := range carts {
            // Release reservations for carts older than 1 hour without activity
            if now-cart.UpdatedAt > 3600 {
                go releaseReservations(cartID)
                cart.Reserved = false
                cart.UpdatedAt = now
                carts[cartID] = cart
            }
        }
        mu.Unlock()
    }
}

func main() {
    // Start cleanup goroutine
    go cleanupExpiredReservations()

    router := mux.NewRouter()

    // API routes
    api := router.PathPrefix("/api/cart").Subrouter()
    api.HandleFunc("/{userId}", getCartHandler).Methods("GET")
    api.HandleFunc("/{userId}/add", addItemHandler).Methods("POST")
    api.HandleFunc("/{userId}/remove/{productId}", removeItemHandler).Methods("DELETE")
    api.HandleFunc("/{userId}/update/{productId}", updateItemHandler).Methods("PUT")
    api.HandleFunc("/{userId}/clear", clearCartHandler).Methods("DELETE")

    // Admin routes
    router.HandleFunc("/admin/clear", clearAllCartsHandler).Methods("DELETE")

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

    port := "8002"
    log.Printf("Cart service starting on port %s", port)
    log.Printf("Inventory service URL: %s", inventoryServiceURL)
    
    if err := http.ListenAndServe(":"+port, handler); err != nil {
        log.Fatal("Server failed to start:", err)
    }
}