# Chains start after acknowledgement

Calcron creates a chained schedule only after the parent delivery is acknowledged by its application. A parent that is pending, disconnected, or retrying blocks its successor, so failed work cannot advance a lifecycle as though it succeeded.
