# Microservices E-commerce Platform

A production-ready microservices e-commerce platform demonstrating advanced architectural patterns, built with Next.js, Go, Node.js, and Python.

## 🏗️ Architecture Overview

This platform showcases a complete microservices architecture with 8 specialized services:

### Frontend
- **Next.js 13+** - Modern React framework with SSR/ISR
- **Tailwind CSS + shadcn/ui** - Beautiful, accessible UI components
- **Real-time service monitoring** - Live health checks and metrics

### API Gateway
- **Traefik** - Intelligent routing, load balancing, and SSL termination
- **Service discovery** - Automatic routing based on Docker labels
- **Rate limiting** - Protection against abuse

### Core Services

#### 1. User Service (Node.js)
- JWT authentication with bcrypt password hashing
- User registration, login, and profile management
- Session management with automatic cleanup
- Rate limiting and security middleware

#### 2. Product Catalog Service (Go)
- High-performance CRUD operations
- Automatic search indexing integration
- Category-based filtering and pagination
- Stock management integration

#### 3. Search & Optimization Service (Python)
- **Advanced data structures:**
  - Inverted index with TF-IDF scoring
  - Autocomplete trie for instant suggestions
  - Bloom filter for existence checks
  - LRU cache for performance
- **Machine learning features:**
  - Collaborative filtering recommendations
  - Category-based similarity
  - Search analytics and query optimization

#### 4. Cart Service (Go)
- Optimistic locking for concurrent access
- Automatic inventory reservations
- Session-based cart persistence
- Reservation cleanup with TTL

#### 5. Inventory Service (Go)
- Atomic stock operations with mutex protection
- Reservation system with expiration
- Optimistic concurrency control
- Stock level monitoring and alerts

#### 6. Order Service (Go)
- Complete order lifecycle management
- Payment processing integration
- Inventory commitment workflow
- Order status tracking and analytics

#### 7. Payment Service (Node.js)
- Stripe integration (mocked for development)
- Multiple payment method support
- Refund processing capabilities
- Transaction history and analytics

#### 8. Notification Service (Python)
- Multi-channel notifications (Email, SMS, Push)
- Template-based messaging system
- Async processing with background tasks
- Delivery tracking and analytics

## 🚀 Advanced Features

### Data Structures & Algorithms
- **Inverted Index**: O(1) search with TF-IDF relevance scoring
- **Trie**: Prefix-based autocomplete with frequency ranking
- **Bloom Filter**: Probabilistic existence checks (99.9% accuracy)
- **LRU Cache**: Memory-efficient product caching
- **Priority Queues**: Notification delivery prioritization

### Communication Patterns
- **REST APIs**: Standard HTTP communication
- **Event-driven**: Async message passing for decoupling
- **Circuit Breaker**: Fault tolerance and resilience
- **Retry Logic**: Automatic recovery from transient failures

### Production Features
- **Health Checks**: Comprehensive service monitoring
- **Metrics**: Prometheus-compatible monitoring
- **Logging**: Structured logging with correlation IDs
- **Security**: JWT authentication, rate limiting, CORS
- **Docker**: Multi-stage builds for optimization
- **Monitoring**: Real-time service health dashboard

## 🛠️ Technology Stack

### Backend Services
- **Go 1.21+**: High-performance services (Cart, Inventory, Orders, Products)
- **Node.js 18+**: Authentication and Payment services
- **Python 3.11+**: AI/ML-powered Search and Notifications
- **Redis**: Caching and session storage

### Frontend & Gateway
- **Next.js 13+**: React with App Router
- **TypeScript**: Type safety throughout
- **Tailwind CSS**: Utility-first styling
- **Traefik v3**: Modern reverse proxy

### Data & Monitoring
- **In-memory stores**: Fast development and testing
- **Prometheus**: Metrics collection
- **Docker Compose**: Local orchestration

## 🏃‍♂️ Quick Start

### Prerequisites
- Docker and Docker Compose
- Node.js 18+ (for local frontend development)

### Launch the Platform

```bash
# Clone the repository
git clone <repository-url>
cd microservices-ecommerce

# Start all services
docker-compose up -d

# Watch logs (optional)
docker-compose logs -f
```

### Access Points
- **Frontend**: http://localhost (Main application)
- **Traefik Dashboard**: http://localhost:8080 (Service routing)
- **Prometheus**: http://localhost:9090 (Metrics)

### API Endpoints
- **Products**: http://localhost/api/products
- **Search**: http://localhost/api/search
- **Cart**: http://localhost/api/cart
- **Orders**: http://localhost/api/orders
- **Inventory**: http://localhost/api/inventory
- **Payments**: http://localhost/api/payments
- **Notifications**: http://localhost/api/notifications
- **Users**: http://localhost/api/users

## 📊 Service Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Frontend      │    │  API Gateway    │    │   Monitoring    │
│   (Next.js)     │◄──►│   (Traefik)     │◄──►│  (Prometheus)   │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                                │
                    ┌───────────┼───────────┐
                    ▼           ▼           ▼
        ┌─────────────────┐ ┌──────────┐ ┌──────────────┐
        │  User Service   │ │ Product  │ │   Search     │
        │   (Node.js)     │ │ Service  │ │  Service     │
        │                 │ │  (Go)    │ │ (Python AI)  │
        └─────────────────┘ └──────────┘ └──────────────┘
                    │           │           │
        ┌─────────────────┐ ┌──────────┐ ┌──────────────┐
        │  Cart Service   │ │Inventory │ │Notification  │
        │     (Go)        │ │ Service  │ │  Service     │
        │                 │ │  (Go)    │ │  (Python)    │
        └─────────────────┘ └──────────┘ └──────────────┘
                    │           │
        ┌─────────────────┐ ┌──────────┐
        │ Order Service   │ │ Payment  │
        │     (Go)        │ │ Service  │
        │                 │ │(Node.js) │
        └─────────────────┘ └──────────┘
```

## 🔧 Development

### Local Development
Each service can be developed independently:

```bash
# Product Service (Go)
cd services/product-service
go run main.go

# Search Service (Python)
cd services/search-service
pip install -r requirements.txt
uvicorn main:app --host 0.0.0.0 --port 8005

# Frontend (Next.js)
npm run dev
```

### Adding New Services
1. Create service directory in `services/`
2. Add Dockerfile and dependencies
3. Update `docker-compose.yml`
4. Add Traefik routing labels
5. Implement health check endpoint
6. Add Prometheus metrics

### Testing
```bash
# Test individual service
curl http://localhost/api/products/health

# Load testing
# Install k6 or use curl for basic testing
```

## 🌟 Key Highlights

### Performance Optimizations
- **Go services**: Concurrent processing with goroutines
- **Python async**: FastAPI with async/await patterns
- **Caching layers**: Multiple levels of caching
- **Connection pooling**: Efficient resource usage

### Scalability Features
- **Stateless services**: Horizontal scaling ready
- **Event-driven**: Loose coupling for independent scaling
- **Circuit breakers**: Graceful degradation
- **Health checks**: Auto-recovery and monitoring

### Security Implementation
- **JWT tokens**: Secure authentication
- **Rate limiting**: DDoS protection
- **CORS policies**: Cross-origin security
- **Input validation**: Data sanitization
- **Secure headers**: Security-first middleware

### Observability
- **Structured logging**: Consistent log formats
- **Metrics collection**: Business and technical metrics
- **Health monitoring**: Real-time service status
- **Error tracking**: Comprehensive error handling

This platform demonstrates enterprise-grade microservices architecture with modern development practices, ready for production deployment on Kubernetes or any container orchestration platform.

## 📈 Monitoring & Analytics

Access the service health dashboard in the frontend to monitor:
- Real-time service status
- Performance metrics
- Business analytics
- Error rates and patterns

## 🤝 Contributing

1. Fork the repository
2. Create feature branches
3. Add comprehensive tests
4. Update documentation
5. Submit pull requests

This architecture serves as a blueprint for building scalable, maintainable microservices applications with modern technologies and best practices.#
