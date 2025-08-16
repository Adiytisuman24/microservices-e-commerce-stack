package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/gorilla/mux"
    "github.com/rs/cors"
)

// OrderItem represents an item in an order
type OrderItem struct {
    ProductID  string `json:"product_id"`
    Quantity   int    `json:"qty"`
    PriceCents int    `json:"price_cents"`
}

// Order represents a customer order
type Order struct {
    OrderID     string      `json:"order_id"`
    UserID      string      `json:"user_id"`
    Items       []OrderItem `json:"items"`
    TotalCents  int         `json:"total_cents"`
    Status      string      `json:"status"` // created, paid, shipped, cancelled
    PaymentID   string      `json:"payment_id"`
    CreatedAt   int64       `json:"created_at"`
    UpdatedAt   int64       `json:"updated_at"`
}

// CreateOrderRequest for creating new orders
type CreateOrderRequest struct {
    CartID        string `json:"cart_id"`
    PaymentMethod string `json:"payment_method"`
}

// PaymentRequest for payment service
type PaymentRequest struct {
    Amount        int    `json:"amount"`
    Currency      string `json:"currency"`
    PaymentMethod string `json:"payment_method"`
    OrderID       string `json:"order_id"`
}

// PaymentResponse from payment service
type PaymentResponse struct {
    Success   bool   `json:"success"`
    PaymentID string `json:"payment_id"`
    Message   string `json:"message"`
}

// NotificationRequest for notification service
type NotificationRequest struct {
    Type      string                 `json:"type"`
    Recipient string                 `json:"recipient"`
    Template  string                 `json:"template"`
    Data      map[string]interface{} `json:"data"`
}

// In-memory order store
var (
    orders   = make(map[string]Order)
    userOrders = make(map[string][]string) // userID -> orderIDs
    mu       sync.RWMutex
)

// Environment variables
var (
    paymentServiceURL      = os.Getenv("PAYMENT_SERVICE_URL")
    inventoryServiceURL    = os.Getenv("INVENTORY_SERVICE_URL")
    notificationServiceURL = os.Getenv("NOTIFICATION_SERVICE_URL")
)

func init() {
    if paymentServiceURL == "" {
        paymentServiceURL = "http://payment-service:3002"
    }
    if inventoryServiceURL == "" {
        inventoryServiceURL = "http://inventory-service:8004"
    }
    if notificationServiceURL == "" {
        notificationServiceURL = "http://notification-service:8006"
    }
}

// Helper function to process payment
func processPayment(orderID string, amount int, currency string, paymentMethod string) (*PaymentResponse, error) {
    if paymentServiceURL == "" {
        return &PaymentResponse{
            Success:   true,
            PaymentID: "mock_payment_" + uuid.New().String()[:8],
            Message:   "Mock payment successful",
        }, nil
    }

    reqData := PaymentRequest{
        Amount:        amount,
        Currency:      currency,
        PaymentMethod: paymentMethod,
        OrderID:       orderID,
    }

    jsonData, err := json.Marshal(reqData)
    if err != nil {
        return nil, err
    }

    resp, err := http.Post(
        paymentServiceURL+"/api/payments/process",
        "application/json",
        bytes.NewBuffer(jsonData),
    )
    if err != nil {
        log.Printf("Failed to call payment service: %v", err)
        return nil, err
    }
    defer resp.Body.Close()

    var paymentResp PaymentResponse
    if err := json.NewDecoder(resp.Body).Decode(&paymentResp); err != nil {
        return nil, err
    }

    return &paymentResp, nil
}

// Helper function to commit inventory reservations
func commitInventoryReservations(cartID string) error {
    if inventoryServiceURL == "" {
        return nil
    }

    // Get cart reservations
    resp, err := http.Get(fmt.Sprintf("%s/api/inventory/cart/%s/reservations", inventoryServiceURL, cartID))
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    var reservationsResp struct {
        Reservations []struct {
            ReservationID string `json:"reservation_id"`
        } `json:"reservations"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&reservationsResp); err != nil {
        return err
    }

    // Commit each reservation
    for _, reservation := range reservationsResp.Reservations {
        commitURL := fmt.Sprintf("%s/api/inventory/commit/%s", inventoryServiceURL, reservation.ReservationID)
        req, _ := http.NewRequest("POST", commitURL, nil)
        
        client := &http.Client{Timeout: 10 * time.Second}
        _, err := client.Do(req)
        if err != nil {
            log.Printf("Failed to commit reservation %s: %v", reservation.ReservationID, err)
            return err
        }
    }

    return nil
}

// Helper function to send notification
func sendNotification(orderID string, userEmail string, template string) {
    if notificationServiceURL == "" {
        return
    }

    notificationReq := NotificationRequest{
        Type:      "email",
        Recipient: userEmail,
        Template:  template,
        Data: map[string]interface{}{
            "order_id": orderID,
            "timestamp": time.Now().Format(time.RFC3339),
        },
    }

    jsonData, err := json.Marshal(notificationReq)
    if err != nil {
        log.Printf("Failed to marshal notification request: %v", err)
        return
    }

    go func() {
        _, err := http.Post(
            notificationServiceURL+"/api/notifications/send",
            "application/json",
            bytes.NewBuffer(jsonData),
        )
        if err != nil {
            log.Printf("Failed to send notification: %v", err)
        }
    }()
}

// Health check endpoint
func healthHandler(w http.ResponseWriter, r *http.Request) {
    mu.RLock()
    orderCount := len(orders)
    mu.RUnlock()

    health := map[string]interface{}{
        "status":      "healthy",
        "service":     "order-service",
        "timestamp":   time.Now().Unix(),
        "order_count": orderCount,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(health)
}

// Create order from cart
func createOrderHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    userID := vars["userId"]

    var req CreateOrderRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    if req.CartID == "" || req.PaymentMethod == "" {
        http.Error(w, "Cart ID and payment method required", http.StatusBadRequest)
        return
    }

    // For MVP, we'll simulate cart data since we don't have direct cart access
    // In production, this would fetch from cart service
    order := Order{
        OrderID:   uuid.New().String(),
        UserID:    userID,
        Items: []OrderItem{
            {ProductID: "sku-12345678", Quantity: 2, PriceCents: 15999},
            {ProductID: "sku-23456789", Quantity: 1, PriceCents: 24999},
        },
        TotalCents: 56997,
        Status:     "created",
        CreatedAt:  time.Now().Unix(),
        UpdatedAt:  time.Now().Unix(),
    }

    // Process payment
    paymentResp, err := processPayment(order.OrderID, order.TotalCents, "USD", req.PaymentMethod)
    if err != nil {
        http.Error(w, "Payment processing failed", http.StatusInternalServerError)
        return
    }

    if !paymentResp.Success {
        http.Error(w, paymentResp.Message, http.StatusBadRequest)
        return
    }

    order.PaymentID = paymentResp.PaymentID
    order.Status = "paid"
    order.UpdatedAt = time.Now().Unix()

    // Commit inventory reservations
    if err := commitInventoryReservations(req.CartID); err != nil {
        log.Printf("Failed to commit inventory for order %s: %v", order.OrderID, err)
        // Continue with order creation but log the error
    }

    // Store order
    mu.Lock()
    orders[order.OrderID] = order
    if userOrders[userID] == nil {
        userOrders[userID] = []string{}
    }
    userOrders[userID] = append(userOrders[userID], order.OrderID)
    mu.Unlock()

    // Send notification (async)
    go sendNotification(order.OrderID, "user@example.com", "order_confirmation")

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(order)
}

// Get order by ID
func getOrderHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    orderID := vars["orderId"]

    mu.RLock()
    order, exists := orders[orderID]
    mu.RUnlock()

    if !exists {
        http.Error(w, "Order not found", http.StatusNotFound)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(order)
}

// Get orders for user
func getUserOrdersHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    userID := vars["userId"]

    mu.RLock()
    orderIDs, exists := userOrders[userID]
    if !exists {
        mu.RUnlock()
        result := map[string]interface{}{
            "orders": []Order{},
            "total":  0,
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(result)
        return
    }

    var userOrderList []Order
    for _, orderID := range orderIDs {
        if order, exists := orders[orderID]; exists {
            userOrderList = append(userOrderList, order)
        }
    }
    mu.RUnlock()

    result := map[string]interface{}{
        "orders": userOrderList,
        "total":  len(userOrderList),
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}

// Update order status
func updateOrderStatusHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    orderID := vars["orderId"]

    var req struct {
        Status string `json:"status"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    validStatuses := map[string]bool{
        "created": true, "paid": true, "shipped": true, "cancelled": true,
    }

    if !validStatuses[req.Status] {
        http.Error(w, "Invalid status", http.StatusBadRequest)
        return
    }

    mu.Lock()
    order, exists := orders[orderID]
    if !exists {
        mu.Unlock()
        http.Error(w, "Order not found", http.StatusNotFound)
        return
    }

    order.Status = req.Status
    order.UpdatedAt = time.Now().Unix()
    orders[orderID] = order
    mu.Unlock()

    // Send status update notification
    if req.Status == "shipped" {
        go sendNotification(order.OrderID, "user@example.com", "order_shipped")
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(order)
}

// Cancel order
func cancelOrderHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    orderID := vars["orderId"]

    mu.Lock()
    order, exists := orders[orderID]
    if !exists {
        mu.Unlock()
        http.Error(w, "Order not found", http.StatusNotFound)
        return
    }

    if order.Status == "shipped" {
        mu.Unlock()
        http.Error(w, "Cannot cancel shipped order", http.StatusBadRequest)
        return
    }

    order.Status = "cancelled"
    order.UpdatedAt = time.Now().Unix()
    orders[orderID] = order
    mu.Unlock()

    // Send cancellation notification
    go sendNotification(order.OrderID, "user@example.com", "order_cancelled")

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(order)
}

// Admin endpoint to clear all orders
func clearOrdersHandler(w http.ResponseWriter, r *http.Request) {
    mu.Lock()
    orders = make(map[string]Order)
    userOrders = make(map[string][]string)
    mu.Unlock()

    result := map[string]string{
        "message": "All orders cleared",
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}

// Get order analytics
func getAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
    mu.RLock()
    defer mu.RUnlock()

    statusCounts := make(map[string]int)
    totalRevenue := 0
    
    for _, order := range orders {
        statusCounts[order.Status]++
        if order.Status == "paid" || order.Status == "shipped" {
            totalRevenue += order.TotalCents
        }
    }

    analytics := map[string]interface{}{
        "total_orders":    len(orders),
        "total_revenue":   totalRevenue,
        "status_breakdown": statusCounts,
        "average_order_value": 0,
    }

    if len(orders) > 0 {
        analytics["average_order_value"] = totalRevenue / len(orders)
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(analytics)
}

// Metrics endpoint
func metricsHandler(w http.ResponseWriter, r *http.Request) {
    mu.RLock()
    defer mu.RUnlock()

    orderCount := len(orders)
    statusCounts := make(map[string]int)
    totalRevenue := 0

    for _, order := range orders {
        statusCounts[order.Status]++
        if order.Status == "paid" || order.Status == "shipped" {
            totalRevenue += order.TotalCents
        }
    }

    metrics := fmt.Sprintf(`
# HELP order_service_orders_total Total number of orders
# TYPE order_service_orders_total counter
order_service_orders_total %d

# HELP order_service_revenue_total Total revenue in cents
# TYPE order_service_revenue_total counter
order_service_revenue_total %d

# HELP order_service_orders_by_status Orders by status
# TYPE order_service_orders_by_status counter
order_service_orders_by_status{status="created"} %d
order_service_orders_by_status{status="paid"} %d
order_service_orders_by_status{status="shipped"} %d
order_service_orders_by_status{status="cancelled"} %d
`, orderCount, totalRevenue, 
   statusCounts["created"], statusCounts["paid"], 
   statusCounts["shipped"], statusCounts["cancelled"])

    w.Header().Set("Content-Type", "text/plain")
    w.Write([]byte(metrics))
}

func main() {
    router := mux.NewRouter()

    // API routes
    api := router.PathPrefix("/api/orders").Subrouter()
    api.HandleFunc("/{userId}", createOrderHandler).Methods("POST")
    api.HandleFunc("/{userId}", getUserOrdersHandler).Methods("GET")
    api.HandleFunc("/{orderId}", getOrderHandler).Methods("GET")
    api.HandleFunc("/{orderId}/status", updateOrderStatusHandler).Methods("PUT")
    api.HandleFunc("/{orderId}/cancel", cancelOrderHandler).Methods("POST")
    api.HandleFunc("/analytics", getAnalyticsHandler).Methods("GET")

    // Admin routes
    router.HandleFunc("/admin/clear", clearOrdersHandler).Methods("DELETE")

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

    port := "8003"
    log.Printf("Order service starting on port %s", port)
    log.Printf("Payment service URL: %s", paymentServiceURL)
    log.Printf("Inventory service URL: %s", inventoryServiceURL)
    log.Printf("Notification service URL: %s", notificationServiceURL)
    
    if err := http.ListenAndServe(":"+port, handler); err != nil {
        log.Fatal("Server failed to start:", err)
    }
}