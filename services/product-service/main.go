package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/gorilla/mux"
    "github.com/rs/cors"
)

// Product represents a product in the catalog
type Product struct {
    ProductID   string            `json:"product_id"`
    Title       string            `json:"title"`
    Description string            `json:"description"`
    Categories  []string          `json:"categories"`
    PriceCents  int               `json:"price_cents"`
    Currency    string            `json:"currency"`
    Images      []string          `json:"images"`
    Stock       int               `json:"stock"`
    Metadata    map[string]interface{} `json:"metadata"`
    CreatedAt   int64             `json:"created_at"`
    UpdatedAt   int64             `json:"updated_at"`
}

// ProductRequest for creating/updating products
type ProductRequest struct {
    Title       string            `json:"title"`
    Description string            `json:"description"`
    Categories  []string          `json:"categories"`
    PriceCents  int               `json:"price_cents"`
    Currency    string            `json:"currency"`
    Images      []string          `json:"images"`
    Stock       int               `json:"stock"`
    Metadata    map[string]interface{} `json:"metadata"`
}

// In-memory product store
var (
    products = make(map[string]Product)
    mu       sync.RWMutex
)

// Environment variables
var (
    searchServiceURL = os.Getenv("SEARCH_SERVICE_URL")
)

func init() {
    if searchServiceURL == "" {
        searchServiceURL = "http://search-service:8005"
    }
}

// Helper function to send product to search service
func indexProductInSearch(product Product) error {
    if searchServiceURL == "" {
        return nil // Skip if search service not configured
    }

    productJSON, err := json.Marshal(product)
    if err != nil {
        return err
    }

    resp, err := http.Post(
        searchServiceURL+"/api/search/index/product",
        "application/json",
        bytes.NewBuffer(productJSON),
    )
    if err != nil {
        log.Printf("Failed to index product in search service: %v", err)
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        log.Printf("Search service returned status: %d", resp.StatusCode)
    }

    return nil
}

// Health check endpoint
func healthHandler(w http.ResponseWriter, r *http.Request) {
    mu.RLock()
    productCount := len(products)
    mu.RUnlock()

    health := map[string]interface{}{
        "status":        "healthy",
        "service":       "product-service",
        "timestamp":     time.Now().Unix(),
        "product_count": productCount,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(health)
}

// Create product
func createProductHandler(w http.ResponseWriter, r *http.Request) {
    var req ProductRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    // Validation
    if req.Title == "" {
        http.Error(w, "Title is required", http.StatusBadRequest)
        return
    }
    if req.PriceCents <= 0 {
        http.Error(w, "Price must be positive", http.StatusBadRequest)
        return
    }
    if req.Currency == "" {
        req.Currency = "USD"
    }

    // Create product
    product := Product{
        ProductID:   "sku-" + uuid.New().String()[:8],
        Title:       req.Title,
        Description: req.Description,
        Categories:  req.Categories,
        PriceCents:  req.PriceCents,
        Currency:    req.Currency,
        Images:      req.Images,
        Stock:       req.Stock,
        Metadata:    req.Metadata,
        CreatedAt:   time.Now().Unix(),
        UpdatedAt:   time.Now().Unix(),
    }

    // Store product
    mu.Lock()
    products[product.ProductID] = product
    mu.Unlock()

    // Index in search service (async)
    go func() {
        if err := indexProductInSearch(product); err != nil {
            log.Printf("Failed to index product %s in search: %v", product.ProductID, err)
        }
    }()

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(product)
}

// Get all products with pagination
func getProductsHandler(w http.ResponseWriter, r *http.Request) {
    // Parse query parameters
    limitStr := r.URL.Query().Get("limit")
    offsetStr := r.URL.Query().Get("offset")
    category := r.URL.Query().Get("category")

    limit := 20 // default
    if limitStr != "" {
        if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
            limit = l
        }
    }

    offset := 0 // default
    if offsetStr != "" {
        if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
            offset = o
        }
    }

    mu.RLock()
    defer mu.RUnlock()

    // Filter and paginate
    var filteredProducts []Product
    for _, product := range products {
        // Category filter
        if category != "" {
            found := false
            for _, cat := range product.Categories {
                if strings.EqualFold(cat, category) {
                    found = true
                    break
                }
            }
            if !found {
                continue
            }
        }
        filteredProducts = append(filteredProducts, product)
    }

    // Pagination
    total := len(filteredProducts)
    start := offset
    if start > total {
        start = total
    }
    end := start + limit
    if end > total {
        end = total
    }

    result := map[string]interface{}{
        "products": filteredProducts[start:end],
        "total":    total,
        "limit":    limit,
        "offset":   offset,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}

// Get single product
func getProductHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    productID := vars["id"]

    mu.RLock()
    product, exists := products[productID]
    mu.RUnlock()

    if !exists {
        http.Error(w, "Product not found", http.StatusNotFound)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(product)
}

// Update product
func updateProductHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    productID := vars["id"]

    mu.Lock()
    product, exists := products[productID]
    if !exists {
        mu.Unlock()
        http.Error(w, "Product not found", http.StatusNotFound)
        return
    }

    var req ProductRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        mu.Unlock()
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    // Update fields
    if req.Title != "" {
        product.Title = req.Title
    }
    if req.Description != "" {
        product.Description = req.Description
    }
    if req.Categories != nil {
        product.Categories = req.Categories
    }
    if req.PriceCents > 0 {
        product.PriceCents = req.PriceCents
    }
    if req.Currency != "" {
        product.Currency = req.Currency
    }
    if req.Images != nil {
        product.Images = req.Images
    }
    if req.Stock >= 0 {
        product.Stock = req.Stock
    }
    if req.Metadata != nil {
        product.Metadata = req.Metadata
    }
    
    product.UpdatedAt = time.Now().Unix()
    products[productID] = product
    mu.Unlock()

    // Update search index (async)
    go func() {
        if err := indexProductInSearch(product); err != nil {
            log.Printf("Failed to update product %s in search: %v", product.ProductID, err)
        }
    }()

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(product)
}

// Delete product
func deleteProductHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    productID := vars["id"]

    mu.Lock()
    _, exists := products[productID]
    if !exists {
        mu.Unlock()
        http.Error(w, "Product not found", http.StatusNotFound)
        return
    }

    delete(products, productID)
    mu.Unlock()

    w.WriteHeader(http.StatusNoContent)
}

// Admin endpoint to clear all products
func clearProductsHandler(w http.ResponseWriter, r *http.Request) {
    mu.Lock()
    products = make(map[string]Product)
    mu.Unlock()

    result := map[string]string{
        "message": "All products cleared",
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}

// Seed some sample products
func seedSampleProducts() {
    sampleProducts := []ProductRequest{
        {
            Title:       "Wireless Bluetooth Headphones",
            Description: "High-quality wireless headphones with noise cancellation and 30-hour battery life",
            Categories:  []string{"audio", "headphones", "wireless"},
            PriceCents:  15999,
            Currency:    "USD",
            Images:      []string{"https://images.pexels.com/photos/3394650/pexels-photo-3394650.jpeg"},
            Stock:       50,
        },
        {
            Title:       "Smart Fitness Watch",
            Description: "Advanced fitness tracker with heart rate monitor, GPS, and sleep tracking",
            Categories:  []string{"wearables", "fitness", "smartwatch"},
            PriceCents:  24999,
            Currency:    "USD",
            Images:      []string{"https://images.pexels.com/photos/1772123/pexels-photo-1772123.jpeg"},
            Stock:       25,
        },
        {
            Title:       "Laptop Stand Adjustable",
            Description: "Ergonomic aluminum laptop stand with adjustable height and angle",
            Categories:  []string{"accessories", "office", "laptop"},
            PriceCents:  4999,
            Currency:    "USD",
            Images:      []string{"https://images.pexels.com/photos/7047046/pexels-photo-7047046.jpeg"},
            Stock:       100,
        },
        {
            Title:       "Gaming Mechanical Keyboard",
            Description: "RGB backlit mechanical keyboard with custom switches for gaming",
            Categories:  []string{"gaming", "keyboard", "peripherals"},
            PriceCents:  12999,
            Currency:    "USD",
            Images:      []string{"https://images.pexels.com/photos/2115257/pexels-photo-2115257.jpeg"},
            Stock:       30,
        },
        {
            Title:       "Wireless Phone Charger",
            Description: "Fast wireless charging pad compatible with all Qi-enabled devices",
            Categories:  []string{"accessories", "charging", "wireless"},
            PriceCents:  2999,
            Currency:    "USD",
            Images:      []string{"https://images.pexels.com/photos/4144112/pexels-photo-4144112.jpeg"},
            Stock:       75,
        },
    }

    for _, req := range sampleProducts {
        product := Product{
            ProductID:   "sku-" + uuid.New().String()[:8],
            Title:       req.Title,
            Description: req.Description,
            Categories:  req.Categories,
            PriceCents:  req.PriceCents,
            Currency:    req.Currency,
            Images:      req.Images,
            Stock:       req.Stock,
            Metadata:    req.Metadata,
            CreatedAt:   time.Now().Unix(),
            UpdatedAt:   time.Now().Unix(),
        }

        products[product.ProductID] = product

        // Index in search service
        go indexProductInSearch(product)
    }

    log.Printf("Seeded %d sample products", len(sampleProducts))
}

// Metrics endpoint
func metricsHandler(w http.ResponseWriter, r *http.Request) {
    mu.RLock()
    productCount := len(products)
    mu.RUnlock()

    metrics := fmt.Sprintf(`
# HELP product_service_products_total Total number of products
# TYPE product_service_products_total counter
product_service_products_total %d
`, productCount)

    w.Header().Set("Content-Type", "text/plain")
    w.Write([]byte(metrics))
}

func main() {
    // Seed sample products
    seedSampleProducts()

    router := mux.NewRouter()

    // API routes
    api := router.PathPrefix("/api/products").Subrouter()
    api.HandleFunc("", createProductHandler).Methods("POST")
    api.HandleFunc("", getProductsHandler).Methods("GET")
    api.HandleFunc("/{id}", getProductHandler).Methods("GET")
    api.HandleFunc("/{id}", updateProductHandler).Methods("PUT")
    api.HandleFunc("/{id}", deleteProductHandler).Methods("DELETE")

    // Admin routes
    router.HandleFunc("/admin/clear", clearProductsHandler).Methods("DELETE")

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

    port := "8001"
    log.Printf("Product service starting on port %s", port)
    log.Printf("Search service URL: %s", searchServiceURL)
    
    if err := http.ListenAndServe(":"+port, handler); err != nil {
        log.Fatal("Server failed to start:", err)
    }
}