# Signals use explicit correlation

A wait-for-signal state names both an event and a correlation key. An incoming signal supplies the same pair, so Calcron advances only matching workflow instances and does not infer business relationships from opaque payloads.
