-- Unify Pounce identity: update builder preset to use gateway avatar and unified description
UPDATE agent_roles SET avatar_path = '/gateway-avatar.png', description = 'Gateway & Builder — Routes conversations, builds tools, dashboards, and agents.' WHERE slug = 'builder' AND is_preset = 1;
