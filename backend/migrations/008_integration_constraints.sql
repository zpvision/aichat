-- Standalone API leads no longer have a non-null collection/site to enforce assistant scope.
ALTER TABLE leads ADD CONSTRAINT lead_assistant_tenant FOREIGN KEY(organization_id,assistant_id) REFERENCES assistants(organization_id,id);
ALTER TABLE leads ADD CONSTRAINT lead_channel_shape CHECK(
 (source='api' AND conversation_id IS NULL AND collection_id IS NULL) OR
 (source IN ('website','staff') AND conversation_id IS NOT NULL AND collection_id IS NOT NULL AND site_id IS NOT NULL)
);
ALTER TABLE webhook_deliveries ADD CONSTRAINT delivery_tenant_id UNIQUE(organization_id,id);
ALTER TABLE webhook_attempts ADD CONSTRAINT attempt_tenant_delivery FOREIGN KEY(organization_id,delivery_id) REFERENCES webhook_deliveries(organization_id,id);
ALTER TABLE webhook_attempts DROP CONSTRAINT webhook_attempts_delivery_id_fkey;
