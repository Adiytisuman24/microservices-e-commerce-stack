from fastapi import FastAPI, HTTPException, Query
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from typing import List, Dict, Optional, Set
import re
import json
import time
from collections import defaultdict, Counter
import heapq
import math
from pybloom_live import BloomFilter
import logging

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(title="Search & Optimization Service", version="1.0.0")

# CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Pydantic models
class Product(BaseModel):
    product_id: str
    title: str
    description: str
    categories: List[str]
    price_cents: int
    currency: str
    images: List[str] = []
    stock: int = 0
    metadata: Dict = {}

class SearchResult(BaseModel):
    product_id: str
    title: str
    score: float
    price_cents: int
    currency: str
    stock: int

class AutocompleteResult(BaseModel):
    suggestions: List[str]
    
class RecommendationResult(BaseModel):
    product_ids: List[str]
    reason: str

# Advanced data structures for optimization
class InvertedIndex:
    def __init__(self):
        self.index = defaultdict(set)
        self.doc_freq = defaultdict(int)
        self.total_docs = 0
        self.doc_lengths = {}
        
    def add_document(self, doc_id: str, text: str, categories: List[str] = None):
        """Add document with TF-IDF scoring support"""
        tokens = self.tokenize(text)
        if categories:
            tokens.extend([f"category:{cat.lower()}" for cat in categories])
            
        # Remove old document if exists
        if doc_id in self.doc_lengths:
            self.remove_document(doc_id)
            
        unique_tokens = set(tokens)
        self.doc_lengths[doc_id] = len(tokens)
        
        for token in unique_tokens:
            if doc_id not in self.index[token]:
                self.index[token].add(doc_id)
                self.doc_freq[token] += 1
                
        self.total_docs = len(self.doc_lengths)
        logger.info(f"Indexed document {doc_id} with {len(unique_tokens)} unique tokens")
        
    def remove_document(self, doc_id: str):
        """Remove document from index"""
        if doc_id not in self.doc_lengths:
            return
            
        # Find all tokens for this document
        tokens_to_remove = []
        for token, doc_set in self.index.items():
            if doc_id in doc_set:
                doc_set.discard(doc_id)
                self.doc_freq[token] -= 1
                if not doc_set:
                    tokens_to_remove.append(token)
                    
        # Clean up empty entries
        for token in tokens_to_remove:
            del self.index[token]
            del self.doc_freq[token]
            
        del self.doc_lengths[doc_id]
        self.total_docs = len(self.doc_lengths)
        
    def search(self, query: str, limit: int = 20) -> List[tuple]:
        """Search with TF-IDF scoring"""
        if not query.strip():
            return []
            
        tokens = self.tokenize(query)
        if not tokens:
            return []
            
        # Calculate document scores
        doc_scores = defaultdict(float)
        
        for token in tokens:
            if token in self.index:
                # Calculate IDF
                df = self.doc_freq[token]
                idf = math.log(self.total_docs / df) if df > 0 else 0
                
                for doc_id in self.index[token]:
                    # Simple TF (can be improved)
                    tf = 1.0 / math.sqrt(self.doc_lengths.get(doc_id, 1))
                    doc_scores[doc_id] += tf * idf
                    
        # Sort by score and return top results
        results = [(doc_id, score) for doc_id, score in doc_scores.items()]
        results.sort(key=lambda x: x[1], reverse=True)
        
        return results[:limit]
        
    def tokenize(self, text: str) -> List[str]:
        """Tokenize text for indexing"""
        if not text:
            return []
        # Convert to lowercase and extract words
        tokens = re.findall(r'\b[a-zA-Z][a-zA-Z0-9]*\b', text.lower())
        return tokens

class TrieNode:
    def __init__(self):
        self.children = {}
        self.is_word = False
        self.frequency = 0

class AutocompleteTrie:
    def __init__(self):
        self.root = TrieNode()
        
    def insert(self, word: str, frequency: int = 1):
        """Insert word into trie with frequency"""
        node = self.root
        word = word.lower()
        
        for char in word:
            if char not in node.children:
                node.children[char] = TrieNode()
            node = node.children[char]
            
        node.is_word = True
        node.frequency += frequency
        
    def search_prefix(self, prefix: str, limit: int = 10) -> List[str]:
        """Find all words with given prefix, sorted by frequency"""
        if not prefix:
            return []
            
        node = self.root
        prefix = prefix.lower()
        
        # Navigate to prefix
        for char in prefix:
            if char not in node.children:
                return []
            node = node.children[char]
            
        # Collect all words from this point
        suggestions = []
        self._collect_words(node, prefix, suggestions)
        
        # Sort by frequency and return top results
        suggestions.sort(key=lambda x: x[1], reverse=True)
        return [word for word, freq in suggestions[:limit]]
        
    def _collect_words(self, node: TrieNode, prefix: str, suggestions: List[tuple]):
        """Recursively collect words from trie node"""
        if node.is_word:
            suggestions.append((prefix, node.frequency))
            
        for char, child in node.children.items():
            self._collect_words(child, prefix + char, suggestions)

class RecommendationEngine:
    def __init__(self):
        self.view_matrix = defaultdict(Counter)  # product_id -> {other_product_id: count}
        self.category_matrix = defaultdict(set)  # category -> {product_ids}
        self.price_ranges = defaultdict(list)    # price_range -> [product_ids]
        
    def record_view(self, product_id: str, session_products: List[str]):
        """Record co-viewed products"""
        for other_id in session_products:
            if other_id != product_id:
                self.view_matrix[product_id][other_id] += 1
                
    def add_product_metadata(self, product_id: str, categories: List[str], price_cents: int):
        """Add product metadata for recommendations"""
        for category in categories:
            self.category_matrix[category.lower()].add(product_id)
            
        # Price ranges: 0-50, 50-100, 100-200, 200-500, 500+
        price_dollars = price_cents / 100
        if price_dollars < 50:
            price_range = "0-50"
        elif price_dollars < 100:
            price_range = "50-100"
        elif price_dollars < 200:
            price_range = "100-200"
        elif price_dollars < 500:
            price_range = "200-500"
        else:
            price_range = "500+"
            
        self.price_ranges[price_range].append(product_id)
        
    def get_recommendations(self, product_id: str, limit: int = 5) -> tuple:
        """Get product recommendations"""
        recommendations = []
        reason = ""
        
        # First try collaborative filtering
        if product_id in self.view_matrix:
            coviewed = self.view_matrix[product_id].most_common(limit)
            if coviewed:
                recommendations = [pid for pid, count in coviewed]
                reason = "frequently viewed together"
                
        # Fallback to category-based recommendations
        if len(recommendations) < limit and product_id in products_store:
            product = products_store[product_id]
            for category in product.get('categories', []):
                similar_products = list(self.category_matrix.get(category.lower(), set()))
                similar_products = [pid for pid in similar_products if pid != product_id]
                recommendations.extend(similar_products)
                
                if len(recommendations) >= limit:
                    recommendations = recommendations[:limit]
                    reason = f"similar products in {category}"
                    break
                    
        return recommendations, reason

# Initialize data structures
inverted_index = InvertedIndex()
autocomplete_trie = AutocompleteTrie()
bloom_filter = BloomFilter(capacity=100000, error_rate=0.001)
recommendation_engine = RecommendationEngine()

# In-memory product store
products_store = {}
search_analytics = defaultdict(int)

# Utility functions
def calculate_relevance_score(product: dict, query_tokens: List[str]) -> float:
    """Calculate relevance score for ranking"""
    score = 0.0
    title_tokens = inverted_index.tokenize(product.get('title', ''))
    desc_tokens = inverted_index.tokenize(product.get('description', ''))
    
    # Title matches get higher weight
    title_matches = len(set(query_tokens) & set(title_tokens))
    score += title_matches * 3.0
    
    # Description matches
    desc_matches = len(set(query_tokens) & set(desc_tokens))
    score += desc_matches * 1.0
    
    # Category matches
    categories = [cat.lower() for cat in product.get('categories', [])]
    category_matches = len(set(query_tokens) & set(categories))
    score += category_matches * 2.0
    
    # Stock bonus (prefer in-stock items)
    if product.get('stock', 0) > 0:
        score += 0.5
        
    return score

@app.get("/health")
async def health_check():
    return {
        "status": "healthy",
        "service": "search-service",
        "timestamp": time.time(),
        "stats": {
            "indexed_products": len(products_store),
            "index_size": inverted_index.total_docs,
            "bloom_filter_capacity": bloom_filter.capacity
        }
    }

@app.post("/api/search/index/product")
async def index_product(product: Product):
    """Index a product for search"""
    try:
        # Store product
        products_store[product.product_id] = product.dict()
        
        # Add to bloom filter
        bloom_filter.add(product.product_id)
        
        # Index for search
        search_text = f"{product.title} {product.description}"
        inverted_index.add_document(
            product.product_id, 
            search_text, 
            product.categories
        )
        
        # Add to autocomplete
        for token in inverted_index.tokenize(product.title):
            if len(token) > 2:  # Only index meaningful tokens
                autocomplete_trie.insert(token)
                
        for category in product.categories:
            autocomplete_trie.insert(category)
            
        # Add to recommendation engine
        recommendation_engine.add_product_metadata(
            product.product_id,
            product.categories,
            product.price_cents
        )
        
        logger.info(f"Successfully indexed product: {product.product_id}")
        return {"status": "indexed", "product_id": product.product_id}
        
    except Exception as e:
        logger.error(f"Error indexing product {product.product_id}: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Indexing failed: {str(e)}")

@app.get("/api/search", response_model=Dict)
async def search_products(
    q: str = Query(..., description="Search query"),
    limit: int = Query(20, ge=1, le=100),
    category: Optional[str] = None,
    min_price: Optional[int] = None,
    max_price: Optional[int] = None
):
    """Search products with advanced filtering"""
    try:
        if not q.strip():
            return {"results": [], "total": 0, "query": q}
            
        # Record search analytics
        search_analytics[q.lower()] += 1
        
        # Get search results from inverted index
        search_results = inverted_index.search(q, limit * 2)  # Get more to allow filtering
        
        # Convert to full product data and apply filters
        results = []
        for product_id, score in search_results:
            if product_id not in products_store:
                continue
                
            product = products_store[product_id]
            
            # Apply filters
            if category and category.lower() not in [cat.lower() for cat in product.get('categories', [])]:
                continue
                
            if min_price and product.get('price_cents', 0) < min_price:
                continue
                
            if max_price and product.get('price_cents', 0) > max_price:
                continue
                
            # Calculate final relevance score
            query_tokens = inverted_index.tokenize(q)
            relevance_score = calculate_relevance_score(product, query_tokens)
            
            results.append(SearchResult(
                product_id=product['product_id'],
                title=product['title'],
                score=relevance_score,
                price_cents=product['price_cents'],
                currency=product['currency'],
                stock=product.get('stock', 0)
            ))
            
            if len(results) >= limit:
                break
        
        # Sort by final relevance score
        results.sort(key=lambda x: x.score, reverse=True)
        
        return {
            "results": results,
            "total": len(results),
            "query": q,
            "filters": {
                "category": category,
                "min_price": min_price,
                "max_price": max_price
            }
        }
        
    except Exception as e:
        logger.error(f"Search error for query '{q}': {str(e)}")
        raise HTTPException(status_code=500, detail=f"Search failed: {str(e)}")

@app.get("/api/search/autocomplete", response_model=AutocompleteResult)
async def autocomplete(
    q: str = Query(..., description="Partial query for autocomplete"),
    limit: int = Query(10, ge=1, le=20)
):
    """Autocomplete search suggestions"""
    try:
        if len(q.strip()) < 2:
            return AutocompleteResult(suggestions=[])
            
        suggestions = autocomplete_trie.search_prefix(q, limit)
        
        return AutocompleteResult(suggestions=suggestions)
        
    except Exception as e:
        logger.error(f"Autocomplete error for query '{q}': {str(e)}")
        raise HTTPException(status_code=500, detail=f"Autocomplete failed: {str(e)}")

@app.get("/api/search/recommendations/{product_id}", response_model=RecommendationResult)
async def get_recommendations(
    product_id: str,
    limit: int = Query(5, ge=1, le=20)
):
    """Get product recommendations"""
    try:
        # Check if product exists
        if product_id not in bloom_filter:
            raise HTTPException(status_code=404, detail="Product not found")
            
        recommendations, reason = recommendation_engine.get_recommendations(product_id, limit)
        
        return RecommendationResult(
            product_ids=recommendations,
            reason=reason
        )
        
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Recommendation error for product {product_id}: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Recommendation failed: {str(e)}")

@app.post("/api/search/analytics/view")
async def record_product_view(data: dict):
    """Record product view for recommendation analytics"""
    try:
        product_id = data.get('product_id')
        session_products = data.get('session_products', [])
        
        if product_id and session_products:
            recommendation_engine.record_view(product_id, session_products)
            
        return {"status": "recorded"}
        
    except Exception as e:
        logger.error(f"Analytics recording error: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Analytics failed: {str(e)}")

@app.get("/api/search/analytics")
async def get_search_analytics():
    """Get search analytics data"""
    try:
        top_searches = sorted(
            search_analytics.items(),
            key=lambda x: x[1],
            reverse=True
        )[:20]
        
        return {
            "top_searches": [{"query": query, "count": count} for query, count in top_searches],
            "total_searches": sum(search_analytics.values()),
            "unique_queries": len(search_analytics),
            "indexed_products": len(products_store)
        }
        
    except Exception as e:
        logger.error(f"Analytics error: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Analytics failed: {str(e)}")

@app.delete("/admin/clear")
async def clear_all_data():
    """Clear all search data (admin endpoint)"""
    try:
        global inverted_index, autocomplete_trie, bloom_filter, recommendation_engine
        global products_store, search_analytics
        
        # Reinitialize all data structures
        inverted_index = InvertedIndex()
        autocomplete_trie = AutocompleteTrie()
        bloom_filter = BloomFilter(capacity=100000, error_rate=0.001)
        recommendation_engine = RecommendationEngine()
        
        products_store.clear()
        search_analytics.clear()
        
        return {"status": "cleared", "message": "All search data has been cleared"}
        
    except Exception as e:
        logger.error(f"Clear data error: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Clear failed: {str(e)}")

@app.get("/metrics")
async def get_metrics():
    """Prometheus metrics endpoint"""
    metrics = f"""
# HELP search_service_products_indexed_total Total number of indexed products
# TYPE search_service_products_indexed_total counter
search_service_products_indexed_total {len(products_store)}

# HELP search_service_total_searches_total Total number of searches performed
# TYPE search_service_total_searches_total counter
search_service_total_searches_total {sum(search_analytics.values())}

# HELP search_service_unique_queries_total Total number of unique search queries
# TYPE search_service_unique_queries_total counter
search_service_unique_queries_total {len(search_analytics)}

# HELP search_service_index_documents_total Total documents in inverted index
# TYPE search_service_index_documents_total counter
search_service_index_documents_total {inverted_index.total_docs}
"""
    
    from fastapi.responses import PlainTextResponse
    return PlainTextResponse(content=metrics, media_type="text/plain")

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8005)