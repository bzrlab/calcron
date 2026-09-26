# Throttle uses leading-edge delivery

The first trigger for a throttled key delivers immediately and starts its cooldown window. Calcron ignores later triggers for that key until the window ends; delayed trailing work uses a normal schedule instead.
