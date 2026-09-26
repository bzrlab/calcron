create table if not exists applications (
  id text primary key,
  namespace text not null unique,
  created_at timestamptz not null default now()
);

create table if not exists application_tokens (
  id text primary key,
  application_id text not null references applications(id),
  secret_hash text not null,
  revoked_at timestamptz,
  created_at timestamptz not null default now()
);
create index if not exists application_tokens_application_id_idx on application_tokens(application_id);

create table if not exists idempotency (
  application_id text not null references applications(id),
  key text not null,
  response jsonb not null,
  created_at timestamptz not null default now(),
  primary key (application_id, key)
);

create table if not exists schedules (
  id text primary key,
  application_id text not null references applications(id),
  schedule_key text not null,
  event text not null,
  payload jsonb not null default '{}'::jsonb,
  run_at timestamptz not null,
  status text not null check (status in ('scheduled','cancelled','delivered')),
  chain jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (application_id, schedule_key)
);
create index if not exists schedules_due_idx on schedules (run_at) where status = 'scheduled';
create index if not exists schedules_application_id_idx on schedules(application_id);

create table if not exists deliveries (
  id text primary key,
  schedule_id text not null references schedules(id),
  application_id text not null references applications(id),
  event text not null,
  payload jsonb not null,
  status text not null check (status in ('pending','acked','blocked','cancelled')),
  attempts integer not null default 0,
  next_attempt_at timestamptz not null,
  locked_until timestamptz,
  lease_owner text,
  last_sent_at timestamptz,
  created_at timestamptz not null default now(),
  acked_at timestamptz
);
create index if not exists deliveries_pending_idx on deliveries (next_attempt_at) where status = 'pending';
create index if not exists deliveries_claim_idx on deliveries (next_attempt_at) where status = 'pending' and locked_until is null;
create index if not exists deliveries_schedule_id_idx on deliveries(schedule_id);
create index if not exists deliveries_application_id_idx on deliveries(application_id);

create table if not exists history (
  id bigserial primary key,
  application_id text not null references applications(id),
  subject_type text not null,
  subject_id text not null,
  event text not null,
  data jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);
create index if not exists history_subject_idx on history(subject_type, subject_id, created_at);

create table if not exists throttles (
  application_id text not null references applications(id),
  throttle_key text not null,
  until_at timestamptz not null,
  primary key (application_id, throttle_key)
);

alter table deliveries add column if not exists locked_until timestamptz;
alter table deliveries add column if not exists lease_owner text;
alter table deliveries alter column schedule_id drop not null;
alter table deliveries add column if not exists workflow_instance_id text;
alter table deliveries add column if not exists next_state text;

create table if not exists workflow_versions (
  application_id text not null references applications(id),
  name text not null,
  version integer not null,
  definition jsonb not null,
  created_at timestamptz not null default now(),
  primary key (application_id,name,version)
);

create table if not exists workflow_instances (
  id text primary key,
  application_id text not null references applications(id),
  workflow_name text not null,
  workflow_version integer not null,
  input jsonb not null default '{}'::jsonb,
  state jsonb not null default '{}'::jsonb,
  current_state text not null,
  status text not null check (status in ('running','waiting_time','waiting_signal','waiting_ack','completed','cancelled')),
  waiting_event text,
  correlation_key text,
  wake_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  foreign key (application_id,workflow_name,workflow_version) references workflow_versions(application_id,name,version)
);
create index if not exists workflow_instances_signal_idx on workflow_instances(application_id,waiting_event,correlation_key) where status='waiting_signal';
create index if not exists workflow_instances_wake_idx on workflow_instances(wake_at) where status='waiting_time';
create index if not exists workflow_instances_application_id_idx on workflow_instances(application_id);
create index if not exists deliveries_workflow_instance_id_idx on deliveries(workflow_instance_id);

create table if not exists calendars (
  application_id text not null references applications(id),
  name text not null,
  definition jsonb not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key (application_id,name)
);

create table if not exists start_schedules (
  id text primary key,
  application_id text not null references applications(id),
  name text not null,
  workflow_name text not null,
  calendar_name text not null,
  local_time text not null,
  missed_policy text not null check (missed_policy in ('skip','run_once_late','catch_up')),
  input jsonb not null default '{}'::jsonb,
  next_at timestamptz not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(application_id,name),
  foreign key(application_id,calendar_name) references calendars(application_id,name)
);
create index if not exists start_schedules_due_idx on start_schedules(next_at);
