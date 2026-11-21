-- legal-ru (legal/consents.md, ui-texts.md): separate consents — PD (consent_version), Demo Terms, optional contact.
ALTER TABLE users ADD COLUMN terms_version TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN terms_at BIGINT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN contact_ok INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN contact_consent_at BIGINT NOT NULL DEFAULT 0;
