'use client';

import { useState, useEffect } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { 
  ShoppingCart, 
  Search, 
  User, 
  Package, 
  CreditCard, 
  Bell,
  TrendingUp,
  Activity,
  Server,
  Database
} from 'lucide-react';

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost';

interface Product {
  product_id: string;
  title: string;
  description: string;
  categories: string[];
  price_cents: number;
  currency: string;
  images: string[];
  stock: number;
}

interface CartItem {
  product_id: string;
  qty: number;
  price_cents: number;
}

interface Cart {
  cart_id: string;
  user_id: string;
  items: CartItem[];
  reserved: boolean;
  updated_at: number;
}

interface ServiceHealth {
  status: string;
  service: string;
  timestamp: number;
  [key: string]: any;
}

export default function Home() {
  const [products, setProducts] = useState<Product[]>([]);
  const [searchResults, setSearchResults] = useState<any[]>([]);
  const [cart, setCart] = useState<Cart | null>(null);
  const [user, setUser] = useState<any>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [loading, setLoading] = useState(false);
  const [serviceHealth, setServiceHealth] = useState<{[key: string]: ServiceHealth}>({});
  const [activeTab, setActiveTab] = useState('products');
  const [alerts, setAlerts] = useState<{type: 'success' | 'error' | 'info'; message: string}[]>([]);

  // Mock user for demonstration
  const mockUser = {
    user_id: 'demo-user-123',
    email: 'demo@example.com',
    name: 'Demo User'
  };

  const addAlert = (type: 'success' | 'error' | 'info', message: string) => {
    const newAlert = { type, message };
    setAlerts(prev => [...prev, newAlert]);
    setTimeout(() => {
      setAlerts(prev => prev.filter(alert => alert !== newAlert));
    }, 5000);
  };

  const fetchProducts = async () => {
    try {
      setLoading(true);
      const response = await fetch(`${API_BASE}/api/products?limit=20`);
      if (response.ok) {
        const data = await response.json();
        setProducts(data.products || []);
      }
    } catch (error) {
      addAlert('error', 'Failed to fetch products');
    } finally {
      setLoading(false);
    }
  };

  const searchProducts = async () => {
    if (!searchQuery.trim()) {
      setSearchResults([]);
      return;
    }

    try {
      setLoading(true);
      const response = await fetch(`${API_BASE}/api/search?q=${encodeURIComponent(searchQuery)}&limit=10`);
      if (response.ok) {
        const data = await response.json();
        setSearchResults(data.results || []);
        addAlert('success', `Found ${data.results?.length || 0} search results`);
      }
    } catch (error) {
      addAlert('error', 'Search failed');
    } finally {
      setLoading(false);
    }
  };

  const fetchCart = async () => {
    if (!mockUser) return;

    try {
      const response = await fetch(`${API_BASE}/api/cart/${mockUser.user_id}`);
      if (response.ok) {
        const data = await response.json();
        setCart(data);
      }
    } catch (error) {
      addAlert('error', 'Failed to fetch cart');
    }
  };

  const addToCart = async (productId: string) => {
    try {
      const response = await fetch(`${API_BASE}/api/cart/${mockUser.user_id}/add`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          product_id: productId,
          qty: 1
        })
      });

      if (response.ok) {
        await fetchCart();
        addAlert('success', 'Product added to cart');
      } else {
        addAlert('error', 'Failed to add to cart');
      }
    } catch (error) {
      addAlert('error', 'Failed to add to cart');
    }
  };

  const createOrder = async () => {
    if (!cart || cart.items.length === 0) {
      addAlert('error', 'Cart is empty');
      return;
    }

    try {
      const response = await fetch(`${API_BASE}/api/orders/${mockUser.user_id}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          cart_id: cart.cart_id,
          payment_method: 'credit_card'
        })
      });

      if (response.ok) {
        const order = await response.json();
        addAlert('success', `Order created successfully! Order ID: ${order.order_id}`);
        await fetchCart(); // Refresh cart
      } else {
        addAlert('error', 'Failed to create order');
      }
    } catch (error) {
      addAlert('error', 'Failed to create order');
    }
  };

  const checkServiceHealth = async () => {
    const services = [
      { name: 'products', url: '/api/products', icon: Package },
      { name: 'search', url: '/api/search', icon: Search },
      { name: 'cart', url: '/api/cart', icon: ShoppingCart },
      { name: 'orders', url: '/api/orders', icon: CreditCard },
      { name: 'inventory', url: '/api/inventory', icon: Database },
      { name: 'payments', url: '/api/payments', icon: CreditCard },
      { name: 'notifications', url: '/api/notifications', icon: Bell },
      { name: 'users', url: '/api/users', icon: User }
    ];

    const healthChecks = services.map(async (service) => {
      try {
        const response = await fetch(`${API_BASE}${service.url.replace('/api/', '/health')}`);
        if (response.ok) {
          const health = await response.json();
          return { [service.name]: { ...health, icon: service.icon } };
        }
      } catch (error) {
        return { 
          [service.name]: { 
            status: 'unhealthy', 
            service: service.name,
            timestamp: Date.now(),
            icon: service.icon,
            error: 'Connection failed'
          } 
        };
      }
      return {};
    });

    const results = await Promise.all(healthChecks);
    const healthStatus = results.reduce((acc, curr) => ({ ...acc, ...curr }), {});
    setServiceHealth(healthStatus);
  };

  useEffect(() => {
    setUser(mockUser);
    fetchProducts();
    fetchCart();
    checkServiceHealth();

    // Refresh health status every 30 seconds
    const interval = setInterval(checkServiceHealth, 30000);
    return () => clearInterval(interval);
  }, []);

  const formatPrice = (cents: number) => {
    return `$${(cents / 100).toFixed(2)}`;
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'healthy': return 'bg-green-500';
      case 'unhealthy': return 'bg-red-500';
      default: return 'bg-yellow-500';
    }
  };

  return (
    <div className="min-h-screen bg-gradient-to-br from-slate-50 to-slate-100">
      {/* Header */}
      <header className="bg-white border-b border-slate-200 shadow-sm">
        <div className="container mx-auto px-4 py-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-4">
              <div className="flex items-center space-x-2">
                <Server className="h-8 w-8 text-blue-600" />
                <h1 className="text-2xl font-bold text-slate-900">E-commerce Microservices</h1>
              </div>
              <Badge variant="outline" className="text-xs">
                Production-Ready Architecture
              </Badge>
            </div>
            
            {user && (
              <div className="flex items-center space-x-4">
                <div className="flex items-center space-x-2">
                  <User className="h-4 w-4 text-slate-600" />
                  <span className="text-sm text-slate-600">{user.name}</span>
                </div>
                {cart && (
                  <Badge variant="outline">
                    <ShoppingCart className="h-3 w-3 mr-1" />
                    {cart.items.length} items
                  </Badge>
                )}
              </div>
            )}
          </div>
        </div>
      </header>

      {/* Alerts */}
      {alerts.length > 0 && (
        <div className="fixed top-20 right-4 z-50 space-y-2">
          {alerts.map((alert, index) => (
            <Alert key={index} className={`w-80 ${
              alert.type === 'success' ? 'border-green-500 bg-green-50' :
              alert.type === 'error' ? 'border-red-500 bg-red-50' :
              'border-blue-500 bg-blue-50'
            }`}>
              <AlertDescription>{alert.message}</AlertDescription>
            </Alert>
          ))}
        </div>
      )}

      <div className="container mx-auto px-4 py-8">
        <Tabs value={activeTab} onValueChange={setActiveTab} className="space-y-6">
          <TabsList className="grid grid-cols-4 w-full max-w-md mx-auto">
            <TabsTrigger value="products">Products</TabsTrigger>
            <TabsTrigger value="search">Search</TabsTrigger>
            <TabsTrigger value="cart">Cart</TabsTrigger>
            <TabsTrigger value="services">Services</TabsTrigger>
          </TabsList>

          {/* Products Tab */}
          <TabsContent value="products" className="space-y-6">
            <div>
              <h2 className="text-3xl font-bold text-slate-900 mb-2">Product Catalog</h2>
              <p className="text-slate-600 mb-6">Browse our collection of products powered by Go microservice</p>
              
              {loading ? (
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
                  {[...Array(8)].map((_, i) => (
                    <Card key={i} className="animate-pulse">
                      <div className="h-48 bg-slate-200"></div>
                      <CardContent className="p-4">
                        <div className="h-4 bg-slate-200 rounded mb-2"></div>
                        <div className="h-3 bg-slate-200 rounded w-2/3"></div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              ) : (
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
                  {products.map((product) => (
                    <Card key={product.product_id} className="group hover:shadow-lg transition-all duration-300">
                      <div className="relative overflow-hidden">
                        <img 
                          src={product.images[0] || 'https://images.pexels.com/photos/607812/pexels-photo-607812.jpeg'} 
                          alt={product.title}
                          className="w-full h-48 object-cover group-hover:scale-105 transition-transform duration-300"
                        />
                        <div className="absolute top-2 right-2">
                          <Badge variant={product.stock > 0 ? 'default' : 'destructive'}>
                            {product.stock > 0 ? `${product.stock} in stock` : 'Out of stock'}
                          </Badge>
                        </div>
                      </div>
                      <CardContent className="p-4">
                        <h3 className="font-semibold text-slate-900 mb-1 line-clamp-2">{product.title}</h3>
                        <p className="text-sm text-slate-600 mb-3 line-clamp-2">{product.description}</p>
                        <div className="flex items-center justify-between">
                          <span className="text-xl font-bold text-blue-600">
                            {formatPrice(product.price_cents)}
                          </span>
                          <Button 
                            size="sm"
                            onClick={() => addToCart(product.product_id)}
                            disabled={product.stock === 0}
                            className="bg-blue-600 hover:bg-blue-700"
                          >
                            <ShoppingCart className="h-4 w-4 mr-1" />
                            Add to Cart
                          </Button>
                        </div>
                        <div className="flex flex-wrap gap-1 mt-2">
                          {product.categories.map((category) => (
                            <Badge key={category} variant="outline" className="text-xs">
                              {category}
                            </Badge>
                          ))}
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}
            </div>
          </TabsContent>

          {/* Search Tab */}
          <TabsContent value="search" className="space-y-6">
            <div>
              <h2 className="text-3xl font-bold text-slate-900 mb-2">Advanced Search</h2>
              <p className="text-slate-600 mb-6">Powered by Python with inverted index, autocomplete, and ML recommendations</p>
              
              <div className="flex space-x-4 mb-6">
                <Input 
                  placeholder="Search products with AI-powered relevance..." 
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="flex-1"
                  onKeyPress={(e) => e.key === 'Enter' && searchProducts()}
                />
                <Button onClick={searchProducts} disabled={loading} className="bg-blue-600 hover:bg-blue-700">
                  <Search className="h-4 w-4 mr-2" />
                  Search
                </Button>
              </div>

              {searchResults.length > 0 && (
                <div>
                  <h3 className="text-xl font-semibold mb-4">Search Results ({searchResults.length})</h3>
                  <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                    {searchResults.map((result) => (
                      <Card key={result.product_id} className="hover:shadow-md transition-shadow">
                        <CardContent className="p-4">
                          <h4 className="font-semibold text-slate-900 mb-2">{result.title}</h4>
                          <div className="flex items-center justify-between mb-2">
                            <span className="text-lg font-bold text-blue-600">
                              {formatPrice(result.price_cents)}
                            </span>
                            <Badge variant="outline">
                              Score: {result.score.toFixed(2)}
                            </Badge>
                          </div>
                          <div className="flex items-center justify-between">
                            <Badge variant={result.stock > 0 ? 'default' : 'destructive'}>
                              {result.stock > 0 ? `${result.stock} available` : 'Out of stock'}
                            </Badge>
                            <Button 
                              size="sm"
                              onClick={() => addToCart(result.product_id)}
                              disabled={result.stock === 0}
                            >
                              Add to Cart
                            </Button>
                          </div>
                        </CardContent>
                      </Card>
                    ))}
                  </div>
                </div>
              )}

              {searchQuery && searchResults.length === 0 && !loading && (
                <div className="text-center py-8">
                  <p className="text-slate-500">No products found for "{searchQuery}"</p>
                </div>
              )}
            </div>
          </TabsContent>

          {/* Cart Tab */}
          <TabsContent value="cart" className="space-y-6">
            <div>
              <h2 className="text-3xl font-bold text-slate-900 mb-2">Shopping Cart</h2>
              <p className="text-slate-600 mb-6">Powered by Go with inventory reservations and optimistic locking</p>
              
              {cart && cart.items.length > 0 ? (
                <div className="space-y-4">
                  <div className="grid gap-4">
                    {cart.items.map((item, index) => (
                      <Card key={`${item.product_id}-${index}`}>
                        <CardContent className="p-4">
                          <div className="flex items-center justify-between">
                            <div>
                              <h4 className="font-semibold">{item.product_id}</h4>
                              <p className="text-sm text-slate-600">Quantity: {item.qty}</p>
                            </div>
                            <div className="text-right">
                              <p className="font-semibold">{formatPrice(item.price_cents * item.qty)}</p>
                              <p className="text-sm text-slate-600">{formatPrice(item.price_cents)} each</p>
                            </div>
                          </div>
                        </CardContent>
                      </Card>
                    ))}
                  </div>

                  <Card className="bg-blue-50 border-blue-200">
                    <CardContent className="p-4">
                      <div className="flex items-center justify-between mb-4">
                        <span className="text-lg font-semibold">Total Items: {cart.items.length}</span>
                        <div className="flex items-center space-x-2">
                          {cart.reserved && (
                            <Badge variant="outline" className="text-green-600 border-green-600">
                              Items Reserved
                            </Badge>
                          )}
                        </div>
                      </div>
                      <Button onClick={createOrder} className="w-full bg-blue-600 hover:bg-blue-700">
                        <CreditCard className="h-4 w-4 mr-2" />
                        Place Order
                      </Button>
                    </CardContent>
                  </Card>
                </div>
              ) : (
                <Card>
                  <CardContent className="p-8 text-center">
                    <ShoppingCart className="h-16 w-16 mx-auto text-slate-300 mb-4" />
                    <h3 className="text-lg font-semibold text-slate-900 mb-2">Your cart is empty</h3>
                    <p className="text-slate-600 mb-4">Add some products to get started</p>
                    <Button onClick={() => setActiveTab('products')}>
                      Browse Products
                    </Button>
                  </CardContent>
                </Card>
              )}
            </div>
          </TabsContent>

          {/* Services Tab */}
          <TabsContent value="services" className="space-y-6">
            <div>
              <h2 className="text-3xl font-bold text-slate-900 mb-2">Service Architecture</h2>
              <p className="text-slate-600 mb-6">Real-time health monitoring of all microservices</p>
              
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                {Object.entries(serviceHealth).map(([serviceName, health]) => {
                  const IconComponent = health.icon || Server;
                  return (
                    <Card key={serviceName} className="relative overflow-hidden">
                      <div className={`absolute top-0 left-0 w-1 h-full ${getStatusColor(health.status)}`}></div>
                      <CardHeader className="pb-2">
                        <CardTitle className="flex items-center space-x-2">
                          <IconComponent className="h-5 w-5" />
                          <span className="capitalize">{serviceName} Service</span>
                        </CardTitle>
                      </CardHeader>
                      <CardContent>
                        <div className="space-y-2">
                          <div className="flex items-center justify-between">
                            <span className="text-sm text-slate-600">Status:</span>
                            <Badge variant={health.status === 'healthy' ? 'default' : 'destructive'}>
                              {health.status}
                            </Badge>
                          </div>
                          
                          {health.timestamp && (
                            <div className="flex items-center justify-between">
                              <span className="text-sm text-slate-600">Last Check:</span>
                              <span className="text-xs text-slate-500">
                                {new Date(health.timestamp * 1000).toLocaleTimeString()}
                              </span>
                            </div>
                          )}

                          {/* Service-specific metrics */}
                          {serviceName === 'products' && health.product_count !== undefined && (
                            <div className="flex items-center justify-between">
                              <span className="text-sm text-slate-600">Products:</span>
                              <span className="text-sm font-medium">{health.product_count}</span>
                            </div>
                          )}

                          {serviceName === 'search' && health.stats && (
                            <div className="space-y-1">
                              <div className="flex items-center justify-between">
                                <span className="text-sm text-slate-600">Indexed:</span>
                                <span className="text-sm font-medium">{health.stats.indexed_products}</span>
                              </div>
                            </div>
                          )}

                          {serviceName === 'cart' && health.cart_count !== undefined && (
                            <div className="flex items-center justify-between">
                              <span className="text-sm text-slate-600">Active Carts:</span>
                              <span className="text-sm font-medium">{health.cart_count}</span>
                            </div>
                          )}

                          {serviceName === 'inventory' && health.inventory_items !== undefined && (
                            <div className="flex items-center justify-between">
                              <span className="text-sm text-slate-600">Items Tracked:</span>
                              <span className="text-sm font-medium">{health.inventory_items}</span>
                            </div>
                          )}

                          {serviceName === 'users' && health.users_count !== undefined && (
                            <div className="flex items-center justify-between">
                              <span className="text-sm text-slate-600">Registered:</span>
                              <span className="text-sm font-medium">{health.users_count}</span>
                            </div>
                          )}

                          {health.error && (
                            <div className="text-xs text-red-600 mt-2">
                              Error: {health.error}
                            </div>
                          )}
                        </div>
                      </CardContent>
                    </Card>
                  );
                })}
              </div>

              <Card className="mt-8">
                <CardHeader>
                  <CardTitle className="flex items-center space-x-2">
                    <Activity className="h-5 w-5" />
                    <span>Architecture Overview</span>
                  </CardTitle>
                  <CardDescription>
                    Production-ready microservices architecture with advanced patterns
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                    <div>
                      <h4 className="font-semibold mb-3">Core Services</h4>
                      <ul className="space-y-2 text-sm">
                        <li className="flex items-center space-x-2">
                          <Badge variant="outline" className="text-xs">Go</Badge>
                          <span>Product Catalog - CRUD with search indexing</span>
                        </li>
                        <li className="flex items-center space-x-2">
                          <Badge variant="outline" className="text-xs">Python</Badge>
                          <span>Search Engine - Inverted index, ML recommendations</span>
                        </li>
                        <li className="flex items-center space-x-2">
                          <Badge variant="outline" className="text-xs">Go</Badge>
                          <span>Cart Service - Inventory reservations</span>
                        </li>
                        <li className="flex items-center space-x-2">
                          <Badge variant="outline" className="text-xs">Go</Badge>
                          <span>Order Service - Workflow orchestration</span>
                        </li>
                        <li className="flex items-center space-x-2">
                          <Badge variant="outline" className="text-xs">Node.js</Badge>
                          <span>Payment Service - Stripe integration</span>
                        </li>
                      </ul>
                    </div>
                    <div>
                      <h4 className="font-semibold mb-3">Architecture Patterns</h4>
                      <ul className="space-y-2 text-sm">
                        <li>• API Gateway (Traefik) for service routing</li>
                        <li>• Event-driven async communication</li>
                        <li>• Circuit breaker and retry patterns</li>
                        <li>• Optimistic locking for inventory</li>
                        <li>• In-memory caching with TTL</li>
                        <li>• Health checks and metrics exposure</li>
                        <li>• Docker containerization</li>
                        <li>• Prometheus monitoring ready</li>
                      </ul>
                    </div>
                  </div>
                </CardContent>
              </Card>
            </div>
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}