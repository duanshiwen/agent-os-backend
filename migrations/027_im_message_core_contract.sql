-- IM message core contract hardening.
-- Aligns backend message storage with conversation-core concepts and sync projection needs.

ALTER TABLE messages ADD COLUMN IF NOT EXISTS reply_to UUID;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS thread_id TEXT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS visibility JSONB;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS client_event_id TEXT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'active';
ALTER TABLE messages ADD COLUMN IF NOT EXISTS edited_at TIMESTAMPTZ;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS deleted_by UUID;

CREATE INDEX IF NOT EXISTS idx_messages_conversation_created_at ON messages(conversation_id, created_at);
CREATE INDEX IF NOT EXISTS idx_messages_reply_to ON messages(reply_to);
CREATE INDEX IF NOT EXISTS idx_messages_thread_id ON messages(thread_id);
CREATE INDEX IF NOT EXISTS idx_messages_status ON messages(status);
CREATE INDEX IF NOT EXISTS idx_messages_deleted_by ON messages(deleted_by);
CREATE INDEX IF NOT EXISTS idx_messages_client_event_id ON messages(client_event_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_sender_client_event_id
    ON messages(sender_id, client_event_id)
    WHERE client_event_id IS NOT NULL AND client_event_id <> '';
