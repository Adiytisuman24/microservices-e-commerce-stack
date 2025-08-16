const express = require('express');
const cors = require('cors');
const helmet = require('helmet');
const rateLimit = require('express-rate-limit');
const { v4: uuidv4 } = require('uuid');
const morgan = require('morgan');

const app = express();
const PORT = process.env.PORT || 3002;
const STRIPE_SECRET_KEY = process.env.STRIPE_SECRET_KEY || 'sk_test_mock_key';

// Stripe configuration (mocked for MVP)
// const stripe = require('stripe')(STRIPE_SECRET_KEY);

// In-memory payment store (for MVP)
const payments = new Map();
const transactions = new Map();

// Middleware
app.use(helmet());
app.use(cors());
app.use(morgan('combined'));
app.use(express.json({ limit: '10mb' }));

// Rate limiting
const limiter = rateLimit({
  windowMs: 15 * 60 * 1000, // 15 minutes
  max: 100 // limit each IP to 100 requests per windowMs
});
app.use('/api/payments', limiter);

// Mock payment methods
const SUPPORTED_PAYMENT_METHODS = [
  'credit_card',
  'debit_card',
  'paypal',
  'apple_pay',
  'google_pay',
  'bank_transfer'
];

// Helper functions
const generatePaymentID = () => {
  return 'pi_' + uuidv4().replace(/-/g, '').substring(0, 24);
};

const validatePaymentAmount = (amount) => {
  return typeof amount === 'number' && amount > 0 && amount <= 999999999; // Max $9,999,999.99
};

const simulatePaymentProcessing = () => {
  // Simulate network delay and occasional failures
  return new Promise((resolve) => {
    const delay = Math.random() * 2000 + 500; // 0.5-2.5s delay
    const success = Math.random() > 0.05; // 95% success rate
    
    setTimeout(() => {
      resolve(success);
    }, delay);
  });
};

// Health check endpoint
app.get('/health', (req, res) => {
  res.json({ 
    status: 'healthy', 
    service: 'payment-service',
    timestamp: new Date().toISOString(),
    payments_processed: payments.size,
    supported_methods: SUPPORTED_PAYMENT_METHODS
  });
});

// Process payment
app.post('/api/payments/process', async (req, res) => {
  try {
    const { amount, currency = 'USD', payment_method, order_id, customer_email } = req.body;

    // Validation
    if (!validatePaymentAmount(amount)) {
      return res.status(400).json({ 
        success: false,
        error: 'Invalid payment amount. Must be between $0.01 and $99,999,999.99' 
      });
    }

    if (!payment_method || !SUPPORTED_PAYMENT_METHODS.includes(payment_method)) {
      return res.status(400).json({ 
        success: false,
        error: 'Invalid or unsupported payment method',
        supported_methods: SUPPORTED_PAYMENT_METHODS
      });
    }

    if (!order_id) {
      return res.status(400).json({ 
        success: false,
        error: 'Order ID is required' 
      });
    }

    // Check if order already has a successful payment
    for (let payment of payments.values()) {
      if (payment.order_id === order_id && payment.status === 'succeeded') {
        return res.status(409).json({
          success: false,
          error: 'Payment already processed for this order'
        });
      }
    }

    const paymentID = generatePaymentID();
    
    // Create payment record
    const payment = {
      payment_id: paymentID,
      amount,
      currency: currency.toUpperCase(),
      payment_method,
      order_id,
      customer_email,
      status: 'processing',
      created_at: Date.now(),
      updated_at: Date.now()
    };

    payments.set(paymentID, payment);

    // Simulate payment processing
    const processingSuccess = await simulatePaymentProcessing();

    if (processingSuccess) {
      // Simulate successful payment with Stripe-like response
      payment.status = 'succeeded';
      payment.processed_at = Date.now();
      payment.stripe_payment_id = 'pi_mock_' + uuidv4().substring(0, 8);
      payment.last_4_digits = Math.floor(Math.random() * 9000) + 1000;
      payment.updated_at = Date.now();

      // Record transaction
      const transaction = {
        transaction_id: 'txn_' + uuidv4().substring(0, 16),
        payment_id: paymentID,
        type: 'payment',
        amount,
        currency: currency.toUpperCase(),
        status: 'completed',
        created_at: Date.now()
      };
      
      transactions.set(transaction.transaction_id, transaction);

      res.json({
        success: true,
        payment_id: paymentID,
        status: 'succeeded',
        amount,
        currency: currency.toUpperCase(),
        message: 'Payment processed successfully',
        transaction_id: transaction.transaction_id,
        processing_time: Date.now() - payment.created_at
      });

    } else {
      // Simulate payment failure
      const errorMessages = [
        'Insufficient funds',
        'Card declined',
        'Invalid card number',
        'Expired card',
        'Security code incorrect',
        'Processing error'
      ];
      
      const errorMessage = errorMessages[Math.floor(Math.random() * errorMessages.length)];
      
      payment.status = 'failed';
      payment.error_message = errorMessage;
      payment.updated_at = Date.now();

      res.status(402).json({
        success: false,
        payment_id: paymentID,
        status: 'failed',
        message: errorMessage,
        error_code: 'payment_failed'
      });
    }

    payments.set(paymentID, payment);

  } catch (error) {
    console.error('Payment processing error:', error);
    res.status(500).json({ 
      success: false,
      error: 'Payment processing failed due to internal error' 
    });
  }
});

// Get payment status
app.get('/api/payments/:paymentId', (req, res) => {
  try {
    const { paymentId } = req.params;
    const payment = payments.get(paymentId);

    if (!payment) {
      return res.status(404).json({ error: 'Payment not found' });
    }

    // Remove sensitive information
    const { stripe_payment_id, ...safePayment } = payment;
    
    res.json({ payment: safePayment });

  } catch (error) {
    console.error('Get payment error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

// Refund payment
app.post('/api/payments/:paymentId/refund', async (req, res) => {
  try {
    const { paymentId } = req.params;
    const { amount: refundAmount, reason = 'requested_by_customer' } = req.body;

    const payment = payments.get(paymentId);

    if (!payment) {
      return res.status(404).json({ 
        success: false,
        error: 'Payment not found' 
      });
    }

    if (payment.status !== 'succeeded') {
      return res.status(400).json({ 
        success: false,
        error: 'Can only refund successful payments' 
      });
    }

    const maxRefundAmount = payment.amount - (payment.refunded_amount || 0);
    const finalRefundAmount = refundAmount || maxRefundAmount;

    if (finalRefundAmount > maxRefundAmount) {
      return res.status(400).json({ 
        success: false,
        error: `Refund amount cannot exceed ${maxRefundAmount} cents` 
      });
    }

    // Simulate refund processing
    const refundSuccess = await simulatePaymentProcessing();

    if (refundSuccess) {
      const refundID = 're_' + uuidv4().substring(0, 20);
      
      // Update payment record
      payment.refunded_amount = (payment.refunded_amount || 0) + finalRefundAmount;
      payment.refund_status = payment.refunded_amount >= payment.amount ? 'fully_refunded' : 'partially_refunded';
      payment.updated_at = Date.now();

      // Create refund transaction
      const refundTransaction = {
        transaction_id: 'txn_' + uuidv4().substring(0, 16),
        payment_id: paymentId,
        refund_id: refundID,
        type: 'refund',
        amount: finalRefundAmount,
        currency: payment.currency,
        status: 'completed',
        reason,
        created_at: Date.now()
      };

      transactions.set(refundTransaction.transaction_id, refundTransaction);
      payments.set(paymentId, payment);

      res.json({
        success: true,
        refund_id: refundID,
        amount: finalRefundAmount,
        currency: payment.currency,
        status: 'succeeded',
        message: 'Refund processed successfully',
        transaction_id: refundTransaction.transaction_id
      });

    } else {
      res.status(500).json({
        success: false,
        error: 'Refund processing failed',
        message: 'Unable to process refund at this time'
      });
    }

  } catch (error) {
    console.error('Refund processing error:', error);
    res.status(500).json({ 
      success: false,
      error: 'Refund processing failed due to internal error' 
    });
  }
});

// Get payment methods
app.get('/api/payments/methods', (req, res) => {
  res.json({
    supported_methods: SUPPORTED_PAYMENT_METHODS.map(method => ({
      id: method,
      name: method.replace('_', ' ').replace(/\b\w/g, l => l.toUpperCase()),
      enabled: true
    }))
  });
});

// Get transaction history
app.get('/api/payments/transactions', (req, res) => {
  try {
    const limit = parseInt(req.query.limit) || 50;
    const offset = parseInt(req.query.offset) || 0;
    const type = req.query.type; // payment, refund

    let transactionList = Array.from(transactions.values());

    // Filter by type
    if (type) {
      transactionList = transactionList.filter(t => t.type === type);
    }

    // Sort by creation date (newest first)
    transactionList.sort((a, b) => b.created_at - a.created_at);

    // Apply pagination
    const paginatedTransactions = transactionList.slice(offset, offset + limit);

    res.json({
      transactions: paginatedTransactions,
      total: transactionList.length,
      limit,
      offset
    });

  } catch (error) {
    console.error('Transaction history error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

// Payment analytics
app.get('/api/payments/analytics', (req, res) => {
  try {
    const paymentList = Array.from(payments.values());
    const transactionList = Array.from(transactions.values());

    const analytics = {
      total_payments: paymentList.length,
      successful_payments: paymentList.filter(p => p.status === 'succeeded').length,
      failed_payments: paymentList.filter(p => p.status === 'failed').length,
      total_revenue: paymentList
        .filter(p => p.status === 'succeeded')
        .reduce((sum, p) => sum + p.amount, 0),
      total_refunded: paymentList
        .reduce((sum, p) => sum + (p.refunded_amount || 0), 0),
      payment_methods: {},
      success_rate: 0
    };

    // Calculate payment method distribution
    paymentList.forEach(payment => {
      if (payment.status === 'succeeded') {
        analytics.payment_methods[payment.payment_method] = 
          (analytics.payment_methods[payment.payment_method] || 0) + 1;
      }
    });

    // Calculate success rate
    if (analytics.total_payments > 0) {
      analytics.success_rate = (analytics.successful_payments / analytics.total_payments) * 100;
    }

    res.json(analytics);

  } catch (error) {
    console.error('Analytics error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

// Admin endpoint to clear payment data
app.delete('/admin/clear', (req, res) => {
  payments.clear();
  transactions.clear();
  res.json({ message: 'All payment data cleared' });
});

// Metrics endpoint
app.get('/metrics', (req, res) => {
  const paymentList = Array.from(payments.values());
  const successfulPayments = paymentList.filter(p => p.status === 'succeeded').length;
  const failedPayments = paymentList.filter(p => p.status === 'failed').length;
  const totalRevenue = paymentList
    .filter(p => p.status === 'succeeded')
    .reduce((sum, p) => sum + p.amount, 0);

  res.set('Content-Type', 'text/plain');
  res.send(`
# HELP payment_service_payments_total Total number of payment attempts
# TYPE payment_service_payments_total counter
payment_service_payments_total ${paymentList.length}

# HELP payment_service_successful_payments_total Total number of successful payments
# TYPE payment_service_successful_payments_total counter
payment_service_successful_payments_total ${successfulPayments}

# HELP payment_service_failed_payments_total Total number of failed payments
# TYPE payment_service_failed_payments_total counter
payment_service_failed_payments_total ${failedPayments}

# HELP payment_service_revenue_total Total revenue in cents
# TYPE payment_service_revenue_total counter
payment_service_revenue_total ${totalRevenue}

# HELP payment_service_transactions_total Total number of transactions
# TYPE payment_service_transactions_total counter
payment_service_transactions_total ${transactions.size}
  `);
});

// Clean up old failed payments (runs every hour)
setInterval(() => {
  const now = Date.now();
  const cleanupAge = 24 * 60 * 60 * 1000; // 24 hours
  
  for (let [paymentId, payment] of payments.entries()) {
    if (payment.status === 'failed' && now - payment.created_at > cleanupAge) {
      payments.delete(paymentId);
    }
  }
}, 60 * 60 * 1000);

app.listen(PORT, () => {
  console.log(`Payment service running on port ${PORT}`);
  console.log(`Stripe integration: ${STRIPE_SECRET_KEY.includes('mock') ? 'MOCKED' : 'ENABLED'}`);
});