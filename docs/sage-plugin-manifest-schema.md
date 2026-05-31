# SAGE Plugin Manifest Schema v1

M4 supports JSON manifests as the review object. Markdown SAGE descriptions may be layered later, but the platform submission contract is JSON-first.

## Required Fields

```json
{
  "sage_version": "1.0",
  "plugin_key": "com.example.hotel-booking",
  "name": "Hotel Booking Assistant",
  "version": "1.0.0",
  "description": "Search and book hotels through an AI agent.",
  "developer": {
    "name": "Example Travel Inc.",
    "website": "https://example.com",
    "support_email": "support@example.com"
  },
  "endpoints": {
    "manifest": "https://example.com/.well-known/sage-plugin.json",
    "flow": "https://example.com/sage/flow",
    "callback": "https://example.com/sage/callback",
    "health": "https://example.com/sage/health"
  },
  "trigger_intents": ["book hotel", "预订酒店"],
  "permissions": [
    {
      "key": "transaction.booking.create",
      "required": true,
      "risk": "high",
      "requires_user_confirmation": true,
      "reason": "Create bookings after explicit user confirmation."
    }
  ],
  "privacy": {
    "data_shared": ["destination", "dates", "budget"],
    "data_retention": "30_days"
  },
  "billing": {
    "model": "free"
  }
}
```

## Validation Rules

- `sage_version`, `plugin_key`, `name`, `version`, `description`, `developer.name`, `endpoints.flow`, and at least one `trigger_intents` item are required.
- `plugin_key` must be reverse-DNS-like lowercase text: `com.example.plugin-name`.
- Endpoint URLs must be absolute `https://` URLs, except localhost is allowed in tests/examples.
- Permissions must use the AgentOS SAGE permission taxonomy.
- High or critical risk permissions must set `requires_user_confirmation=true`.
- Manifest hash is computed from normalized JSON with stable key ordering.

## Review Snapshot

The backend stores a manifest snapshot per submitted version. Runtime flows are not stored as review objects; they are reported through invocation reports and may be sampled later.
