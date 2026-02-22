-- Rename API key setting from Anthropic to OpenRouter
UPDATE settings SET key = 'openrouter_api_key' WHERE key = 'anthropic_api_key';

-- Migrate model short names to OpenRouter IDs in settings
UPDATE settings SET value = 'anthropic/claude-haiku-4-5'
  WHERE key IN ('gateway_model', 'builder_model') AND value = 'haiku';
UPDATE settings SET value = 'anthropic/claude-sonnet-4-6'
  WHERE key IN ('gateway_model', 'builder_model') AND value = 'sonnet';
UPDATE settings SET value = 'anthropic/claude-opus-4-6'
  WHERE key IN ('gateway_model', 'builder_model') AND value = 'opus';

-- Migrate agent_roles model column
UPDATE agent_roles SET model = 'anthropic/claude-haiku-4-5' WHERE model = 'haiku';
UPDATE agent_roles SET model = 'anthropic/claude-sonnet-4-6' WHERE model = 'sonnet';
UPDATE agent_roles SET model = 'anthropic/claude-opus-4-6' WHERE model = 'opus';

-- Migrate agents history table
UPDATE agents SET model = 'anthropic/claude-haiku-4-5' WHERE model = 'haiku';
UPDATE agents SET model = 'anthropic/claude-sonnet-4-6' WHERE model = 'sonnet';
UPDATE agents SET model = 'anthropic/claude-opus-4-6' WHERE model = 'opus';

-- Add image_url column to chat_messages for generated images
ALTER TABLE chat_messages ADD COLUMN image_url TEXT DEFAULT NULL;
