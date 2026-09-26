# Declarative generic workflows

Calcron runs versioned, declarative workflow definitions rather than application-specific code. A workflow instance can wait for time, a signal, a condition, or a business-calendar rule, then emit a command or create another wait; applications supply data and perform outside effects.

## Consequences

The product is a durable workflow engine with scheduling, not a timer API with built-in MFS or payment behavior. Definitions must be inspectable and safe to run, so Calcron will not execute arbitrary Go or JavaScript supplied by applications.
