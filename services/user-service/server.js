const express = require('express');
const jwt = require('jsonwebtoken');
const bcrypt = require('bcrypt');
const cors = require('cors');
const helmet = require('helmet');
const rateLimit = require('express-rate-limit');
const { v4: uuidv4 } = require('uuid');
const morgan = require('morgan');

const app = express();
const PORT = process.env.PORT || 3001;
const JWT_SECRET = process.env.JWT_SECRET || 'your-secret-key-here';

// In-memory user store (for MVP)
const users = new Map();
const sessions = new Map();

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
app.use('/api/users', limiter);

// Auth middleware
const authenticateToken = (req, res, next) => {
  const authHeader = req.headers['authorization'];
  const token = authHeader && authHeader.split(' ')[1];

  if (!token) {
    return res.status(401).json({ error: 'Access token required' });
  }

  jwt.verify(token, JWT_SECRET, (err, user) => {
    if (err) {
      return res.status(403).json({ error: 'Invalid or expired token' });
    }
    req.user = user;
    next();
  });
};

// Helper functions
const hashPassword = async (password) => {
  return await bcrypt.hash(password, 10);
};

const validatePassword = async (password, hash) => {
  return await bcrypt.compare(password, hash);
};

const generateToken = (user) => {
  return jwt.sign(
    { 
      user_id: user.user_id, 
      email: user.email,
      name: user.name 
    },
    JWT_SECRET,
    { expiresIn: '24h' }
  );
};

// Health check
app.get('/health', (req, res) => {
  res.json({ 
    status: 'healthy', 
    service: 'user-service',
    timestamp: new Date().toISOString(),
    users_count: users.size
  });
});

// Register user
app.post('/api/users/register', async (req, res) => {
  try {
    const { email, password, name } = req.body;

    if (!email || !password || !name) {
      return res.status(400).json({ 
        error: 'Email, password, and name are required' 
      });
    }

    // Check if user already exists
    for (let user of users.values()) {
      if (user.email === email) {
        return res.status(409).json({ error: 'User already exists' });
      }
    }

    const hashedPassword = await hashPassword(password);
    const user = {
      user_id: uuidv4(),
      email,
      password_hash: hashedPassword,
      name,
      addresses: [],
      created_at: Date.now(),
      roles: ['customer']
    };

    users.set(user.user_id, user);

    const token = generateToken(user);
    sessions.set(user.user_id, { token, created_at: Date.now() });

    const { password_hash, ...safeUser } = user;
    res.status(201).json({ 
      user: safeUser, 
      token,
      expires_in: '24h'
    });

  } catch (error) {
    console.error('Registration error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

// Login user
app.post('/api/users/login', async (req, res) => {
  try {
    const { email, password } = req.body;

    if (!email || !password) {
      return res.status(400).json({ 
        error: 'Email and password are required' 
      });
    }

    // Find user by email
    let user = null;
    for (let u of users.values()) {
      if (u.email === email) {
        user = u;
        break;
      }
    }

    if (!user) {
      return res.status(401).json({ error: 'Invalid credentials' });
    }

    const isValidPassword = await validatePassword(password, user.password_hash);
    if (!isValidPassword) {
      return res.status(401).json({ error: 'Invalid credentials' });
    }

    const token = generateToken(user);
    sessions.set(user.user_id, { token, created_at: Date.now() });

    const { password_hash, ...safeUser } = user;
    res.json({ 
      user: safeUser, 
      token,
      expires_in: '24h'
    });

  } catch (error) {
    console.error('Login error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

// Get user profile
app.get('/api/users/profile', authenticateToken, (req, res) => {
  try {
    const user = users.get(req.user.user_id);
    if (!user) {
      return res.status(404).json({ error: 'User not found' });
    }

    const { password_hash, ...safeUser } = user;
    res.json({ user: safeUser });

  } catch (error) {
    console.error('Profile error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

// Update user profile
app.put('/api/users/profile', authenticateToken, (req, res) => {
  try {
    const user = users.get(req.user.user_id);
    if (!user) {
      return res.status(404).json({ error: 'User not found' });
    }

    const { name, addresses } = req.body;
    
    if (name) user.name = name;
    if (addresses && Array.isArray(addresses)) user.addresses = addresses;
    
    users.set(user.user_id, user);

    const { password_hash, ...safeUser } = user;
    res.json({ user: safeUser });

  } catch (error) {
    console.error('Profile update error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

// Logout
app.post('/api/users/logout', authenticateToken, (req, res) => {
  try {
    sessions.delete(req.user.user_id);
    res.json({ message: 'Logged out successfully' });
  } catch (error) {
    console.error('Logout error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

// Admin endpoint to clear data
app.delete('/admin/clear', (req, res) => {
  users.clear();
  sessions.clear();
  res.json({ message: 'All user data cleared' });
});

// Metrics endpoint
app.get('/metrics', (req, res) => {
  res.set('Content-Type', 'text/plain');
  res.send(`
# HELP user_service_users_total Total number of registered users
# TYPE user_service_users_total counter
user_service_users_total ${users.size}

# HELP user_service_active_sessions_total Total number of active sessions
# TYPE user_service_active_sessions_total counter
user_service_active_sessions_total ${sessions.size}
  `);
});

// Cleanup expired sessions (runs every hour)
setInterval(() => {
  const now = Date.now();
  const expiry = 24 * 60 * 60 * 1000; // 24 hours
  
  for (let [userId, session] of sessions.entries()) {
    if (now - session.created_at > expiry) {
      sessions.delete(userId);
    }
  }
}, 60 * 60 * 1000);

app.listen(PORT, () => {
  console.log(`User service running on port ${PORT}`);
});