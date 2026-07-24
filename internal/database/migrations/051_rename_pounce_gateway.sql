-- The builder/gateway preset was renamed from "Pounce" to "Gateway". New installs
-- already seed "Gateway" (internal/agents/preset_roles.go), but existing databases
-- keep the old stored name. Update any lingering "Pounce" to "Gateway".
UPDATE agent_roles SET name = 'Gateway' WHERE slug = 'builder' AND is_preset = 1 AND name = 'Pounce';
