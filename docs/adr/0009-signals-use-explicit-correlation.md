# Signals use explicit correlation

A wait-for-signal state names both an event and a correlation key. The key is a literal or a CEL expression over the instance's own input and state, fixed when the instance starts waiting, so concurrent instances of one definition wait on distinct keys. An incoming signal supplies the same pair, so Calcron advances only matching workflow instances and does not infer business relationships from opaque payloads.
