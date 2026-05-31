-- M4 SAGE Plugin asset bindings.

ALTER TABLE sage_plugin_versions
    ADD COLUMN IF NOT EXISTS package_object_id UUID;

CREATE INDEX IF NOT EXISTS idx_sage_plugins_icon_object ON sage_plugins(icon_object_id);
CREATE INDEX IF NOT EXISTS idx_sage_plugin_versions_package_object ON sage_plugin_versions(package_object_id);
