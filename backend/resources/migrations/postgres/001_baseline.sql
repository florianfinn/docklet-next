-- Source: docklet-next/backend/internal/schema/models.go
-- Revision: be847e36ab81fdf0cd6a7dad333046ddea990e71
-- Retrieved: 2026-10-13
-- License: Apache-2.0
-- Deviations: Generated PostgreSQL baseline from the retained GORM model registry.
-- Generated from backend/internal/schema.Models with PostgreSQL 18 pg_dump.
-- +goose Up
CREATE TABLE public.activities (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    environment_id text NOT NULL,
    batch_id text,
    type text NOT NULL,
    status text NOT NULL,
    resource_type text,
    resource_id text,
    resource_name text,
    progress bigint,
    step text,
    latest_message text,
    started_by_user_id text,
    started_by_username text,
    started_by_display_name text,
    started_at timestamp with time zone NOT NULL,
    ended_at timestamp with time zone,
    duration_ms bigint,
    error text,
    metadata text
);
CREATE TABLE public.activity_messages (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    activity_id text NOT NULL,
    level text NOT NULL,
    message text NOT NULL,
    payload text
);
CREATE TABLE public.api_key_permissions (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    api_key_id text NOT NULL,
    permission text NOT NULL,
    environment_id text
);
CREATE TABLE public.api_keys (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name text NOT NULL,
    description text,
    key_hash text NOT NULL,
    key_prefix text NOT NULL,
    managed_by text,
    kind text DEFAULT 'scoped'::text NOT NULL,
    user_id text,
    environment_id text,
    expires_at timestamp with time zone,
    last_used_at timestamp with time zone
);
CREATE TABLE public.auto_update_records (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    resource_id text,
    resource_type text,
    resource_name text,
    status text,
    start_time timestamp with time zone,
    end_time timestamp with time zone,
    update_available boolean,
    update_applied boolean,
    old_image_versions text,
    new_image_versions text,
    error text,
    details text
);
CREATE TABLE public.container_registries (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    url text,
    username text,
    token text,
    description text,
    insecure boolean,
    enabled boolean,
    registry_type text,
    repository_names text DEFAULT '[]'::text NOT NULL,
    aws_access_key_id text,
    aws_secret_access_key text,
    aws_region text,
    ecr_token text,
    ecr_token_generated_at timestamp with time zone
);
CREATE TABLE public.environment_update_jobs (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    status text,
    user_id text,
    username text,
    manager_version_at_start text,
    manager_digest_at_start text,
    manager_target_version text,
    results text,
    error text,
    completed_at timestamp with time zone
);
CREATE TABLE public.environments (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name text,
    api_url text,
    status text,
    enabled boolean,
    is_edge boolean DEFAULT false,
    last_seen timestamp with time zone,
    last_edge_transport text,
    access_token text,
    api_key_id text
);
CREATE TABLE public.events (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deduplication_key text,
    type text,
    severity text,
    title text,
    description text,
    resource_type text,
    resource_id text,
    resource_name text,
    user_id text,
    username text,
    environment_id text,
    metadata text,
    "timestamp" timestamp with time zone
);
CREATE TABLE public.image_patches (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    environment_id text,
    original_image_id text,
    original_ref text,
    original_digest text,
    patched_ref text,
    mode text,
    status text,
    packages_updated bigint,
    error text,
    activity_id text,
    duration_ms bigint
);
CREATE TABLE public.image_updates (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    project_id text DEFAULT ''::text NOT NULL,
    service_name text DEFAULT ''::text NOT NULL,
    policy_key text DEFAULT ''::text NOT NULL,
    container_id text DEFAULT ''::text NOT NULL,
    image_id text DEFAULT ''::text NOT NULL,
    check_time timestamp with time zone,
    latest_version text,
    current_digest text,
    latest_digest text,
    last_error text,
    auth_method text,
    auth_username text,
    auth_registry text,
    repository text,
    tag text,
    update_type text,
    current_version text,
    response_time_ms bigint,
    has_update boolean,
    used_credential boolean,
    notification_sent boolean DEFAULT false
);
CREATE TABLE public.kv (
    key text NOT NULL,
    value text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE public.notification_settings (
    id bigint NOT NULL,
    provider character varying(50) NOT NULL,
    enabled boolean DEFAULT false,
    config jsonb,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE SEQUENCE public.notification_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.notification_settings_id_seq OWNED BY public.notification_settings.id;
CREATE TABLE public.project_tags (
    project_id text NOT NULL,
    name text NOT NULL,
    source text NOT NULL,
    color text DEFAULT 'gray'::text NOT NULL
);
CREATE TABLE public.projects (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name text,
    dir_name text,
    path text,
    status text,
    status_reason text,
    service_count bigint,
    running_count bigint,
    compose_project_name text,
    image_refs_json text,
    build_image_refs_json text,
    is_archived boolean DEFAULT false,
    archived_at timestamp with time zone
);
CREATE TABLE public.roles (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name text NOT NULL,
    description text,
    permissions text NOT NULL,
    built_in boolean DEFAULT false NOT NULL
);
CREATE TABLE public.settings (
    key text NOT NULL,
    value text
);
CREATE TABLE public.system_backup_policies (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    enabled boolean DEFAULT false NOT NULL,
    schedule text NOT NULL,
    retention_count bigint DEFAULT 7 NOT NULL,
    local_enabled boolean DEFAULT true NOT NULL
);
CREATE TABLE public.system_backup_recovery_config (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    encrypted_recovery_key text NOT NULL
);
CREATE TABLE public.system_backup_runs (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    size bigint,
    status text NOT NULL,
    trigger text NOT NULL,
    destination text NOT NULL,
    local_snapshot_id text,
    policy_id text,
    error text
);
CREATE TABLE public.user_avatars (
    user_id text NOT NULL,
    data bytea NOT NULL,
    mime_type text NOT NULL
);
CREATE TABLE public.user_role_assignments (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    user_id text NOT NULL,
    role_id text NOT NULL,
    environment_id text,
    source text DEFAULT 'manual'::text NOT NULL
);
CREATE TABLE public.user_sessions (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    user_id text NOT NULL,
    refresh_token_hash text NOT NULL,
    user_agent text,
    ip_address text,
    source text,
    last_used_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone
);
CREATE TABLE public.users (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    username text,
    password_hash text,
    display_name text,
    email text,
    last_login timestamp with time zone,
    locale text,
    time_format text DEFAULT 'auto'::text NOT NULL,
    font_size bigint,
    preferences text,
    requires_password_change boolean,
    has_avatar boolean DEFAULT false NOT NULL
);
CREATE TABLE public.volume_backup_policies (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    volume_name text NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    schedule text NOT NULL,
    retention_count bigint DEFAULT 7 NOT NULL,
    stop_containers boolean DEFAULT false NOT NULL,
    local_enabled boolean NOT NULL
);
CREATE TABLE public.volume_backups (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    volume_name text,
    size bigint,
    status text DEFAULT 'succeeded'::text NOT NULL,
    trigger text DEFAULT 'manual'::text NOT NULL,
    destination text DEFAULT 'local'::text NOT NULL,
    format text DEFAULT 'archive'::text NOT NULL,
    local_snapshot_id text,
    policy_id text,
    error text
);
CREATE TABLE public.vulnerability_ignores (
    id text NOT NULL,
    environment_id text,
    image_id text,
    vulnerability_id text,
    pkg_name text,
    installed_version text,
    reason text,
    created_by text,
    created_at timestamp with time zone
);
CREATE TABLE public.vulnerability_reports (
    image_id text NOT NULL,
    data text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE public.vulnerability_risk_snapshots (
    snapshot_date text NOT NULL,
    scoring_version bigint,
    risk_score bigint,
    risk_band text,
    critical_count bigint,
    high_count bigint,
    medium_count bigint,
    low_count bigint,
    unknown_count bigint,
    findings_count bigint,
    known_exploited_count bigint,
    overdue_known_exploited_count bigint,
    high_epss_count bigint,
    exposed_critical_high_count bigint,
    fixable_count bigint,
    images_scanned bigint,
    images_total bigint,
    scored_images bigint,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE public.vulnerability_scan_items (
    id bigint NOT NULL,
    image_id text,
    vulnerability_id text,
    pkg_name text,
    installed_version text,
    fixed_version text,
    severity text,
    severity_rank bigint,
    pkg_class text,
    pkg_type text,
    title text,
    cvss_score numeric,
    details text
);
CREATE SEQUENCE public.vulnerability_scan_items_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.vulnerability_scan_items_id_seq OWNED BY public.vulnerability_scan_items.id;
CREATE TABLE public.vulnerability_scans (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    image_name text,
    status text,
    scan_time timestamp with time zone,
    duration bigint,
    critical_count bigint,
    high_count bigint,
    medium_count bigint,
    low_count bigint,
    unknown_count bigint,
    total_count bigint,
    vulnerabilities text,
    error text,
    scanner_version text,
    fixable_count bigint,
    scan_policy text
);
CREATE TABLE public.vulnerability_threat_intel (
    cve_id text NOT NULL,
    known_exploited boolean,
    kev_date_added text,
    kev_due_date text,
    kev_ransomware boolean,
    epss_score numeric,
    epss_percentile numeric,
    epss_checked_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE public.webhooks (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name text NOT NULL,
    token_hash text NOT NULL,
    token_prefix text NOT NULL,
    target_type text NOT NULL,
    action_type text DEFAULT ''::text NOT NULL,
    target_id text NOT NULL,
    target_ref text DEFAULT ''::text NOT NULL,
    environment_id text DEFAULT ''::text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    last_triggered_at timestamp with time zone
);
ALTER TABLE ONLY public.notification_settings ALTER COLUMN id SET DEFAULT nextval('public.notification_settings_id_seq'::regclass);
ALTER TABLE ONLY public.vulnerability_scan_items ALTER COLUMN id SET DEFAULT nextval('public.vulnerability_scan_items_id_seq'::regclass);
ALTER TABLE ONLY public.activities
    ADD CONSTRAINT activities_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.activity_messages
    ADD CONSTRAINT activity_messages_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.api_key_permissions
    ADD CONSTRAINT api_key_permissions_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.auto_update_records
    ADD CONSTRAINT auto_update_records_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.container_registries
    ADD CONSTRAINT container_registries_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.environment_update_jobs
    ADD CONSTRAINT environment_update_jobs_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.environments
    ADD CONSTRAINT environments_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.events
    ADD CONSTRAINT events_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.image_patches
    ADD CONSTRAINT image_patches_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.image_updates
    ADD CONSTRAINT image_updates_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.kv
    ADD CONSTRAINT kv_pkey PRIMARY KEY (key);
ALTER TABLE ONLY public.notification_settings
    ADD CONSTRAINT notification_settings_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.project_tags
    ADD CONSTRAINT project_tags_pkey PRIMARY KEY (project_id, name, source);
ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.settings
    ADD CONSTRAINT settings_pkey PRIMARY KEY (key);
ALTER TABLE ONLY public.system_backup_policies
    ADD CONSTRAINT system_backup_policies_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.system_backup_recovery_config
    ADD CONSTRAINT system_backup_recovery_config_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.system_backup_runs
    ADD CONSTRAINT system_backup_runs_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.user_avatars
    ADD CONSTRAINT user_avatars_pkey PRIMARY KEY (user_id);
ALTER TABLE ONLY public.user_role_assignments
    ADD CONSTRAINT user_role_assignments_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.user_sessions
    ADD CONSTRAINT user_sessions_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.volume_backup_policies
    ADD CONSTRAINT volume_backup_policies_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.volume_backups
    ADD CONSTRAINT volume_backups_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.vulnerability_ignores
    ADD CONSTRAINT vulnerability_ignores_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.vulnerability_reports
    ADD CONSTRAINT vulnerability_reports_pkey PRIMARY KEY (image_id);
ALTER TABLE ONLY public.vulnerability_risk_snapshots
    ADD CONSTRAINT vulnerability_risk_snapshots_pkey PRIMARY KEY (snapshot_date);
ALTER TABLE ONLY public.vulnerability_scan_items
    ADD CONSTRAINT vulnerability_scan_items_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.vulnerability_scans
    ADD CONSTRAINT vulnerability_scans_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.vulnerability_threat_intel
    ADD CONSTRAINT vulnerability_threat_intel_pkey PRIMARY KEY (cve_id);
ALTER TABLE ONLY public.webhooks
    ADD CONSTRAINT webhooks_pkey PRIMARY KEY (id);
CREATE INDEX idx_activities_batch_id ON public.activities USING btree (batch_id);
CREATE INDEX idx_activities_environment_id ON public.activities USING btree (environment_id);
CREATE INDEX idx_activities_resource_id ON public.activities USING btree (resource_id);
CREATE INDEX idx_activities_resource_type ON public.activities USING btree (resource_type);
CREATE INDEX idx_activities_started_by_user_id ON public.activities USING btree (started_by_user_id);
CREATE INDEX idx_activities_status ON public.activities USING btree (status);
CREATE INDEX idx_activities_type ON public.activities USING btree (type);
CREATE INDEX idx_activity_messages_activity_id ON public.activity_messages USING btree (activity_id);
CREATE INDEX idx_api_key_permissions_api_key_id ON public.api_key_permissions USING btree (api_key_id);
CREATE UNIQUE INDEX idx_events_deduplication_key ON public.events USING btree (deduplication_key);
CREATE INDEX idx_image_patches_env_image ON public.image_patches USING btree (environment_id, original_image_id);
CREATE INDEX idx_image_updates_container_id ON public.image_updates USING btree (container_id);
CREATE INDEX idx_image_updates_image_id ON public.image_updates USING btree (image_id);
CREATE INDEX idx_image_updates_project_id ON public.image_updates USING btree (project_id);
CREATE INDEX idx_notification_settings_provider ON public.notification_settings USING btree (provider);
CREATE INDEX idx_projects_is_archived ON public.projects USING btree (is_archived);
CREATE INDEX idx_projects_name ON public.projects USING btree (name);
CREATE UNIQUE INDEX idx_projects_path ON public.projects USING btree (path);
CREATE UNIQUE INDEX idx_roles_name ON public.roles USING btree (name);
CREATE INDEX idx_system_backup_runs_policy_id ON public.system_backup_runs USING btree (policy_id);
CREATE INDEX idx_user_role_assignments_environment_id ON public.user_role_assignments USING btree (environment_id);
CREATE INDEX idx_user_role_assignments_role_id ON public.user_role_assignments USING btree (role_id);
CREATE INDEX idx_user_role_assignments_user_id ON public.user_role_assignments USING btree (user_id);
CREATE INDEX idx_user_sessions_expires_at ON public.user_sessions USING btree (expires_at);
CREATE UNIQUE INDEX idx_user_sessions_refresh_token_hash ON public.user_sessions USING btree (refresh_token_hash);
CREATE INDEX idx_user_sessions_user_id ON public.user_sessions USING btree (user_id);
CREATE INDEX idx_volume_backup_policies_volume_name ON public.volume_backup_policies USING btree (volume_name);
CREATE INDEX idx_volume_backups_policy_id ON public.volume_backups USING btree (policy_id);
CREATE INDEX idx_volume_backups_volume_name ON public.volume_backups USING btree (volume_name);
CREATE INDEX idx_vulnerability_ignores_environment_id ON public.vulnerability_ignores USING btree (environment_id);
CREATE INDEX idx_vulnerability_ignores_image_id ON public.vulnerability_ignores USING btree (image_id);
CREATE INDEX idx_vulnerability_ignores_vulnerability_id ON public.vulnerability_ignores USING btree (vulnerability_id);
CREATE INDEX idx_vulnerability_scan_items_image_id ON public.vulnerability_scan_items USING btree (image_id);
CREATE INDEX idx_vulnerability_scan_items_severity ON public.vulnerability_scan_items USING btree (severity);
CREATE INDEX idx_vulnerability_scan_items_vulnerability_id ON public.vulnerability_scan_items USING btree (vulnerability_id);
CREATE UNIQUE INDEX idx_webhooks_token_hash ON public.webhooks USING btree (token_hash);
