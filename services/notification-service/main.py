from fastapi import FastAPI, HTTPException, BackgroundTasks
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, EmailStr
from typing import Dict, List, Optional, Any
import logging
import time
import json
import asyncio
from collections import defaultdict, deque
import os

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(title="Notification Service", version="1.0.0")

# CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Environment variables
SENDGRID_API_KEY = os.getenv("SENDGRID_API_KEY", "mock_key")
TWILIO_SID = os.getenv("TWILIO_SID", "mock_sid")
TWILIO_AUTH_TOKEN = os.getenv("TWILIO_AUTH_TOKEN", "mock_token")

# Pydantic models
class NotificationRequest(BaseModel):
    type: str  # email, sms, push
    recipient: str
    template: str
    data: Dict[str, Any] = {}
    priority: str = "normal"  # low, normal, high, urgent

class EmailNotification(BaseModel):
    to: str
    subject: str
    body: str
    html_body: Optional[str] = None
    template: Optional[str] = None
    template_data: Dict[str, Any] = {}

class SMSNotification(BaseModel):
    to: str
    message: str
    template: Optional[str] = None
    template_data: Dict[str, Any] = {}

class NotificationStatus(BaseModel):
    notification_id: str
    type: str
    recipient: str
    status: str  # pending, sent, delivered, failed
    created_at: float
    sent_at: Optional[float] = None
    error_message: Optional[str] = None

# In-memory stores
notifications_history = {}
failed_notifications = deque(maxlen=1000)  # Keep last 1000 failed notifications
notification_templates = {}
notification_stats = defaultdict(int)

# Initialize email templates
EMAIL_TEMPLATES = {
    "welcome": {
        "subject": "Welcome to our E-commerce Platform!",
        "body": "Hi {name},\n\nWelcome to our platform! We're excited to have you on board.\n\nBest regards,\nThe E-commerce Team",
        "html_body": """
        <html>
        <body>
            <h2>Welcome to our E-commerce Platform!</h2>
            <p>Hi {name},</p>
            <p>Welcome to our platform! We're excited to have you on board.</p>
            <p>Best regards,<br>The E-commerce Team</p>
        </body>
        </html>
        """
    },
    "order_confirmation": {
        "subject": "Order Confirmation - #{order_id}",
        "body": "Hi there,\n\nYour order #{order_id} has been confirmed and is being processed.\n\nOrder Date: {timestamp}\n\nThank you for your purchase!\n\nBest regards,\nThe E-commerce Team",
        "html_body": """
        <html>
        <body>
            <h2>Order Confirmation</h2>
            <p>Hi there,</p>
            <p>Your order <strong>#{order_id}</strong> has been confirmed and is being processed.</p>
            <p>Order Date: {timestamp}</p>
            <p>Thank you for your purchase!</p>
            <p>Best regards,<br>The E-commerce Team</p>
        </body>
        </html>
        """
    },
    "order_shipped": {
        "subject": "Your Order Has Shipped - #{order_id}",
        "body": "Hi there,\n\nGreat news! Your order #{order_id} has been shipped and is on its way to you.\n\nShipped Date: {timestamp}\n\nYou should receive it within 3-5 business days.\n\nBest regards,\nThe E-commerce Team",
        "html_body": """
        <html>
        <body>
            <h2>Your Order Has Shipped!</h2>
            <p>Hi there,</p>
            <p>Great news! Your order <strong>#{order_id}</strong> has been shipped and is on its way to you.</p>
            <p>Shipped Date: {timestamp}</p>
            <p>You should receive it within 3-5 business days.</p>
            <p>Best regards,<br>The E-commerce Team</p>
        </body>
        </html>
        """
    },
    "order_cancelled": {
        "subject": "Order Cancellation - #{order_id}",
        "body": "Hi there,\n\nYour order #{order_id} has been cancelled as requested.\n\nCancellation Date: {timestamp}\n\nIf you have any questions, please contact our support team.\n\nBest regards,\nThe E-commerce Team",
        "html_body": """
        <html>
        <body>
            <h2>Order Cancellation</h2>
            <p>Hi there,</p>
            <p>Your order <strong>#{order_id}</strong> has been cancelled as requested.</p>
            <p>Cancellation Date: {timestamp}</p>
            <p>If you have any questions, please contact our support team.</p>
            <p>Best regards,<br>The E-commerce Team</p>
        </body>
        </html>
        """
    },
    "password_reset": {
        "subject": "Password Reset Request",
        "body": "Hi {name},\n\nWe received a request to reset your password.\n\nIf you didn't request this, please ignore this email.\n\nBest regards,\nThe E-commerce Team",
        "html_body": """
        <html>
        <body>
            <h2>Password Reset Request</h2>
            <p>Hi {name},</p>
            <p>We received a request to reset your password.</p>
            <p>If you didn't request this, please ignore this email.</p>
            <p>Best regards,<br>The E-commerce Team</p>
        </body>
        </html>
        """
    }
}

# Initialize SMS templates
SMS_TEMPLATES = {
    "order_confirmation": "Your order #{order_id} has been confirmed! Thank you for your purchase. - E-commerce Team",
    "order_shipped": "Good news! Your order #{order_id} has shipped and is on its way. Expected delivery in 3-5 business days.",
    "order_cancelled": "Your order #{order_id} has been cancelled as requested. Contact support if you have questions.",
    "promotional": "Hi {name}! Don't miss our special offer: {offer_text}. Shop now and save!",
    "verification": "Your verification code is: {code}. This code expires in 10 minutes."
}

def generate_notification_id() -> str:
    """Generate unique notification ID"""
    import uuid
    return f"notif_{int(time.time())}_{uuid.uuid4().hex[:8]}"

def render_template(template_content: str, data: Dict[str, Any]) -> str:
    """Simple template rendering with variable substitution"""
    try:
        return template_content.format(**data)
    except KeyError as e:
        logger.warning(f"Template variable missing: {e}")
        return template_content
    except Exception as e:
        logger.error(f"Template rendering error: {e}")
        return template_content

async def simulate_email_delivery(recipient: str, subject: str, body: str) -> bool:
    """Simulate email delivery with realistic delays and failure rates"""
    # Simulate processing delay
    await asyncio.sleep(0.5 + (len(body) / 1000))  # Longer emails take more time
    
    # Simulate 5% failure rate
    import random
    success = random.random() > 0.05
    
    if success:
        logger.info(f"✅ Email sent successfully to {recipient}: {subject}")
    else:
        logger.error(f"❌ Email delivery failed to {recipient}: {subject}")
    
    return success

async def simulate_sms_delivery(recipient: str, message: str) -> bool:
    """Simulate SMS delivery with realistic delays and failure rates"""
    # Simulate processing delay
    await asyncio.sleep(0.2 + (len(message) / 500))
    
    # Simulate 3% failure rate (SMS generally more reliable)
    import random
    success = random.random() > 0.03
    
    if success:
        logger.info(f"✅ SMS sent successfully to {recipient}: {message[:50]}...")
    else:
        logger.error(f"❌ SMS delivery failed to {recipient}: {message[:50]}...")
    
    return success

@app.get("/health")
async def health_check():
    return {
        "status": "healthy",
        "service": "notification-service",
        "timestamp": time.time(),
        "stats": {
            "total_notifications": len(notifications_history),
            "failed_notifications": len(failed_notifications),
            "email_templates": len(EMAIL_TEMPLATES),
            "sms_templates": len(SMS_TEMPLATES)
        }
    }

@app.post("/api/notifications/send")
async def send_notification(
    notification: NotificationRequest,
    background_tasks: BackgroundTasks
):
    """Send notification (email, SMS, or push)"""
    try:
        notification_id = generate_notification_id()
        
        # Create notification status record
        status = NotificationStatus(
            notification_id=notification_id,
            type=notification.type,
            recipient=notification.recipient,
            status="pending",
            created_at=time.time()
        )
        
        notifications_history[notification_id] = status.dict()
        notification_stats["total"] += 1
        notification_stats[f"type_{notification.type}"] += 1
        
        # Process notification based on type
        if notification.type == "email":
            background_tasks.add_task(
                process_email_notification,
                notification_id,
                notification.recipient,
                notification.template,
                notification.data
            )
        elif notification.type == "sms":
            background_tasks.add_task(
                process_sms_notification,
                notification_id,
                notification.recipient,
                notification.template,
                notification.data
            )
        elif notification.type == "push":
            background_tasks.add_task(
                process_push_notification,
                notification_id,
                notification.recipient,
                notification.template,
                notification.data
            )
        else:
            raise HTTPException(status_code=400, detail="Unsupported notification type")
        
        return {
            "success": True,
            "notification_id": notification_id,
            "status": "queued",
            "message": f"{notification.type.title()} notification queued for delivery"
        }
        
    except Exception as e:
        logger.error(f"Error sending notification: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Notification failed: {str(e)}")

async def process_email_notification(
    notification_id: str,
    recipient: str,
    template: str,
    data: Dict[str, Any]
):
    """Process email notification"""
    try:
        # Get template
        if template not in EMAIL_TEMPLATES:
            raise ValueError(f"Email template '{template}' not found")
        
        template_data = EMAIL_TEMPLATES[template]
        subject = render_template(template_data["subject"], data)
        body = render_template(template_data["body"], data)
        html_body = render_template(template_data["html_body"], data)
        
        # Simulate sending email
        success = await simulate_email_delivery(recipient, subject, body)
        
        # Update notification status
        status = notifications_history[notification_id]
        status["sent_at"] = time.time()
        
        if success:
            status["status"] = "delivered"
            notification_stats["delivered"] += 1
        else:
            status["status"] = "failed"
            status["error_message"] = "Email delivery failed"
            notification_stats["failed"] += 1
            failed_notifications.append({
                "notification_id": notification_id,
                "type": "email",
                "recipient": recipient,
                "error": "Email delivery failed",
                "timestamp": time.time()
            })
        
        notifications_history[notification_id] = status
        
    except Exception as e:
        logger.error(f"Email processing error: {str(e)}")
        status = notifications_history[notification_id]
        status["status"] = "failed"
        status["error_message"] = str(e)
        notifications_history[notification_id] = status
        notification_stats["failed"] += 1

async def process_sms_notification(
    notification_id: str,
    recipient: str,
    template: str,
    data: Dict[str, Any]
):
    """Process SMS notification"""
    try:
        # Get template
        if template not in SMS_TEMPLATES:
            raise ValueError(f"SMS template '{template}' not found")
        
        message = render_template(SMS_TEMPLATES[template], data)
        
        # Validate message length (SMS limit)
        if len(message) > 160:
            logger.warning(f"SMS message too long ({len(message)} chars), truncating")
            message = message[:157] + "..."
        
        # Simulate sending SMS
        success = await simulate_sms_delivery(recipient, message)
        
        # Update notification status
        status = notifications_history[notification_id]
        status["sent_at"] = time.time()
        
        if success:
            status["status"] = "delivered"
            notification_stats["delivered"] += 1
        else:
            status["status"] = "failed"
            status["error_message"] = "SMS delivery failed"
            notification_stats["failed"] += 1
            failed_notifications.append({
                "notification_id": notification_id,
                "type": "sms",
                "recipient": recipient,
                "error": "SMS delivery failed",
                "timestamp": time.time()
            })
        
        notifications_history[notification_id] = status
        
    except Exception as e:
        logger.error(f"SMS processing error: {str(e)}")
        status = notifications_history[notification_id]
        status["status"] = "failed"
        status["error_message"] = str(e)
        notifications_history[notification_id] = status
        notification_stats["failed"] += 1

async def process_push_notification(
    notification_id: str,
    recipient: str,
    template: str,
    data: Dict[str, Any]
):
    """Process push notification (mock implementation)"""
    try:
        # For MVP, push notifications are mocked
        await asyncio.sleep(0.1)  # Simulate processing
        
        # Mock success (95% success rate)
        import random
        success = random.random() > 0.05
        
        status = notifications_history[notification_id]
        status["sent_at"] = time.time()
        
        if success:
            status["status"] = "delivered"
            notification_stats["delivered"] += 1
            logger.info(f"📱 Push notification sent to {recipient}")
        else:
            status["status"] = "failed"
            status["error_message"] = "Push notification delivery failed"
            notification_stats["failed"] += 1
            logger.error(f"📱 Push notification failed for {recipient}")
        
        notifications_history[notification_id] = status
        
    except Exception as e:
        logger.error(f"Push notification error: {str(e)}")
        status = notifications_history[notification_id]
        status["status"] = "failed"
        status["error_message"] = str(e)
        notifications_history[notification_id] = status
        notification_stats["failed"] += 1

@app.get("/api/notifications/{notification_id}")
async def get_notification_status(notification_id: str):
    """Get notification status"""
    if notification_id not in notifications_history:
        raise HTTPException(status_code=404, detail="Notification not found")
    
    return notifications_history[notification_id]

@app.get("/api/notifications/history")
async def get_notification_history(
    limit: int = 50,
    offset: int = 0,
    type_filter: Optional[str] = None,
    status_filter: Optional[str] = None
):
    """Get notification history with filters"""
    try:
        history_list = list(notifications_history.values())
        
        # Apply filters
        if type_filter:
            history_list = [n for n in history_list if n["type"] == type_filter]
        
        if status_filter:
            history_list = [n for n in history_list if n["status"] == status_filter]
        
        # Sort by creation time (newest first)
        history_list.sort(key=lambda x: x["created_at"], reverse=True)
        
        # Apply pagination
        paginated_history = history_list[offset:offset + limit]
        
        return {
            "notifications": paginated_history,
            "total": len(history_list),
            "limit": limit,
            "offset": offset
        }
        
    except Exception as e:
        logger.error(f"History retrieval error: {str(e)}")
        raise HTTPException(status_code=500, detail=f"History retrieval failed: {str(e)}")

@app.get("/api/notifications/templates")
async def get_templates():
    """Get available notification templates"""
    return {
        "email_templates": list(EMAIL_TEMPLATES.keys()),
        "sms_templates": list(SMS_TEMPLATES.keys()),
        "templates": {
            "email": {name: {"subject": template["subject"]} for name, template in EMAIL_TEMPLATES.items()},
            "sms": {name: {"preview": template[:50] + "..."} for name, template in SMS_TEMPLATES.items()}
        }
    }

@app.post("/api/notifications/test")
async def send_test_notification(test_data: dict):
    """Send test notification for development"""
    try:
        notification = NotificationRequest(
            type=test_data.get("type", "email"),
            recipient=test_data.get("recipient", "test@example.com"),
            template=test_data.get("template", "welcome"),
            data=test_data.get("data", {"name": "Test User"})
        )
        
        # Process immediately for testing
        notification_id = generate_notification_id()
        
        if notification.type == "email":
            await process_email_notification(
                notification_id,
                notification.recipient,
                notification.template,
                notification.data
            )
        elif notification.type == "sms":
            await process_sms_notification(
                notification_id,
                notification.recipient,
                notification.template,
                notification.data
            )
        
        return {
            "success": True,
            "notification_id": notification_id,
            "message": "Test notification processed",
            "status": notifications_history.get(notification_id, {}).get("status", "unknown")
        }
        
    except Exception as e:
        logger.error(f"Test notification error: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Test failed: {str(e)}")

@app.get("/api/notifications/analytics")
async def get_notification_analytics():
    """Get notification analytics and statistics"""
    try:
        # Calculate delivery rates
        total = notification_stats.get("total", 0)
        delivered = notification_stats.get("delivered", 0)
        failed = notification_stats.get("failed", 0)
        
        delivery_rate = (delivered / total * 100) if total > 0 else 0
        failure_rate = (failed / total * 100) if total > 0 else 0
        
        # Recent activity (last 24 hours)
        now = time.time()
        day_ago = now - (24 * 60 * 60)
        recent_notifications = [
            n for n in notifications_history.values() 
            if n["created_at"] > day_ago
        ]
        
        return {
            "overview": {
                "total_notifications": total,
                "delivered": delivered,
                "failed": failed,
                "pending": total - delivered - failed,
                "delivery_rate": round(delivery_rate, 2),
                "failure_rate": round(failure_rate, 2)
            },
            "by_type": {
                "email": notification_stats.get("type_email", 0),
                "sms": notification_stats.get("type_sms", 0),
                "push": notification_stats.get("type_push", 0)
            },
            "recent_activity": {
                "last_24h": len(recent_notifications),
                "recent_failures": len([n for n in recent_notifications if n["status"] == "failed"])
            },
            "templates_used": {
                template: len([n for n in notifications_history.values() if template in str(n)])
                for template in list(EMAIL_TEMPLATES.keys()) + list(SMS_TEMPLATES.keys())
            }
        }
        
    except Exception as e:
        logger.error(f"Analytics error: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Analytics failed: {str(e)}")

@app.delete("/admin/clear")
async def clear_notification_data():
    """Clear all notification data (admin endpoint)"""
    try:
        global notifications_history, failed_notifications, notification_stats
        
        notifications_history.clear()
        failed_notifications.clear()
        notification_stats.clear()
        
        return {
            "success": True,
            "message": "All notification data cleared"
        }
        
    except Exception as e:
        logger.error(f"Clear data error: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Clear failed: {str(e)}")

@app.get("/metrics")
async def get_metrics():
    """Prometheus metrics endpoint"""
    total = notification_stats.get("total", 0)
    delivered = notification_stats.get("delivered", 0)
    failed = notification_stats.get("failed", 0)
    
    metrics = f"""
# HELP notification_service_notifications_total Total notifications sent
# TYPE notification_service_notifications_total counter
notification_service_notifications_total {total}

# HELP notification_service_delivered_total Total notifications delivered
# TYPE notification_service_delivered_total counter
notification_service_delivered_total {delivered}

# HELP notification_service_failed_total Total notifications failed
# TYPE notification_service_failed_total counter
notification_service_failed_total {failed}

# HELP notification_service_email_total Total email notifications
# TYPE notification_service_email_total counter
notification_service_email_total {notification_stats.get("type_email", 0)}

# HELP notification_service_sms_total Total SMS notifications
# TYPE notification_service_sms_total counter
notification_service_sms_total {notification_stats.get("type_sms", 0)}

# HELP notification_service_push_total Total push notifications
# TYPE notification_service_push_total counter
notification_service_push_total {notification_stats.get("type_push", 0)}
"""
    
    from fastapi.responses import PlainTextResponse
    return PlainTextResponse(content=metrics, media_type="text/plain")

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8006)