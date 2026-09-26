# Blocked deliveries after capped retries

Calcron keeps delivery for a disconnected application queued without counting failure. A connected application that fails to acknowledge receives capped exponential retries; after five failed attempts the delivery becomes blocked, remains inspectable, and requires administrator replay or cancellation.
