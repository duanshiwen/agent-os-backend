# SAGE Plugin Permission Model

SAGE Plugin permissions describe what a plugin may ask AgentOS Client to do or disclose during runtime.

## Permission Categories

### Context

- `context.profile.read`
- `context.memory.read`
- `context.calendar.read`
- `context.location.read`
- `context.files.read`
- `context.contacts.read`

### Communication

- `communication.message.draft`
- `communication.message.send`
- `communication.email.draft`
- `communication.email.send`
- `communication.social.publish`

### Transaction

- `transaction.booking.create`
- `transaction.order.create`
- `transaction.payment.initiate`
- `transaction.subscription.create`

### Local Action

- `local.file.write`
- `local.browser.open`
- `local.browser.operate`
- `local.notification.schedule`
- `local.task.schedule`

### Plugin Network

- `plugin.api.call`
- `plugin.callback.send`
- `third_party.api.call`

## Risk Levels

| Risk | Policy |
|---|---|
| `low` | May be granted during install. |
| `medium` | Requires explicit install-time grant. |
| `high` | Requires sensitive confirmation for grant and runtime guard. |
| `critical` | Requires strongest confirmation; M4 allows recording but policy bundle denies by default unless explicitly granted. |

## Runtime Governance

Backend produces policy bundles. AgentOS Client enforces them before executing plugin flow steps.

Policy bundle decisions include:

- `allow`
- `require_user_confirmation`
- `deny`

Backend M4 does not execute plugin actions; it defines and records governance state.
