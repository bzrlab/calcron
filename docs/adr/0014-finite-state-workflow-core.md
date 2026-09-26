# Finite-state workflow core

Calcron interprets workflow definitions as finite-state machines with `wait_time`, `wait_signal`, `branch`, `emit`, and `end` states. Loops use explicit transitions; direct scheduler operations remain separate from the workflow interpreter.
