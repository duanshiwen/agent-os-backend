# SAGE Plugin Runtime Contract v1

This contract is between AgentOS Client and third-party SAGE Plugin Servers. AgentOS Backend stores manifest, permissions, policy bundles, and execution reports, but does not run the flow in M4.

## Flow Request

```json
{
  "request_id": "req_123",
  "user_intent": "帮我找东京酒店",
  "locale": "zh-CN",
  "timezone": "Asia/Shanghai",
  "user_constraints": {
    "destination": "Tokyo"
  },
  "available_agent_capabilities": [
    "llm.generate",
    "user.ask",
    "user.confirm",
    "http.call"
  ],
  "granted_permissions": ["plugin.api.call"],
  "privacy_context": {
    "allow_profile_sharing": false,
    "allow_precise_location": false
  }
}
```

## Flow Response

```json
{
  "flow_id": "flow_123",
  "plugin_key": "com.example.hotel-booking",
  "title": "Search Tokyo Hotels",
  "risk_level": "medium",
  "requires_permissions": ["plugin.api.call"],
  "steps": [
    {
      "id": "search_hotels",
      "type": "plugin_api",
      "method": "POST",
      "path": "/api/hotels/search",
      "input": {"destination": "Tokyo"},
      "output_key": "hotel_candidates"
    },
    {
      "id": "present_options",
      "type": "present_to_user",
      "input": {"items": "{{steps.search_hotels.output}}"}
    }
  ],
  "reporting": {
    "callback_required": true
  }
}
```

## Step Types v1

- `plugin_api`
- `llm_generate`
- `ask_user`
- `present_to_user`
- `user_confirm`
- `callback_plugin`
- `stop`

## Execution Reporting

AgentOS Client reports invocation and execution summaries back to AgentOS Backend for audit, quality metrics, and usage ledger.
