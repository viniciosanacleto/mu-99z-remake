-- Core schema. Blob columns keep the exact byte layout of the original
-- MuOnline database (docs/server-plan/06-schema-postgresql.md).
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE accounts (
  id               bigserial PRIMARY KEY,
  login            citext UNIQUE NOT NULL CHECK (length(login) BETWEEN 1 AND 10),
  password_hash    text   NOT NULL,
  personal_id      varchar(18) NOT NULL DEFAULT '',
  email            varchar(50),
  block_code       smallint NOT NULL DEFAULT 0,
  account_level    smallint NOT NULL DEFAULT 0,
  level_expires_at timestamptz,
  lock_code        integer NOT NULL DEFAULT 0,
  ext_class        integer NOT NULL DEFAULT 0,
  move_count       integer NOT NULL DEFAULT 0,
  credits          bigint  NOT NULL DEFAULT 0 CHECK (credits >= 0),
  gold             bigint  NOT NULL DEFAULT 0,
  created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE account_sessions (
  account_id      bigint PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  online          boolean NOT NULL DEFAULT false,
  server_name     varchar(50),
  ip              varchar(20),
  connected_at    timestamptz,
  disconnected_at timestamptz,
  online_hours    integer NOT NULL DEFAULT 0
);

CREATE TABLE default_class_templates (
  class smallint PRIMARY KEY,
  level integer NOT NULL, level_up_points integer NOT NULL,
  strength integer NOT NULL, dexterity integer NOT NULL, vitality integer NOT NULL,
  energy integer NOT NULL, leadership integer NOT NULL,
  inventory bytea NOT NULL, magic_list bytea NOT NULL,
  life real NOT NULL, max_life real NOT NULL, mana real NOT NULL, max_mana real NOT NULL,
  map smallint NOT NULL, pos_x smallint NOT NULL, pos_y smallint NOT NULL,
  quest bytea NOT NULL, effect_list bytea NOT NULL
);

CREATE TABLE characters (
  id               bigserial PRIMARY KEY,
  account_id       bigint NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  slot             smallint NOT NULL CHECK (slot BETWEEN 0 AND 4),
  name             citext UNIQUE NOT NULL CHECK (length(name) BETWEEN 1 AND 10),
  class            smallint NOT NULL,
  level            integer NOT NULL DEFAULT 1,
  level_up_points  integer NOT NULL DEFAULT 0,
  experience       bigint  NOT NULL DEFAULT 0,
  strength integer NOT NULL, dexterity integer NOT NULL, vitality integer NOT NULL,
  energy   integer NOT NULL, leadership integer NOT NULL DEFAULT 0,
  money            bigint  NOT NULL DEFAULT 0,
  life real NOT NULL, max_life real NOT NULL, mana real NOT NULL, max_mana real NOT NULL,
  bp real NOT NULL DEFAULT 0, max_bp real NOT NULL DEFAULT 0,
  shield real NOT NULL DEFAULT 0, max_shield real NOT NULL DEFAULT 0,
  map smallint NOT NULL, pos_x smallint NOT NULL, pos_y smallint NOT NULL, dir smallint NOT NULL DEFAULT 0,
  pk_count integer NOT NULL DEFAULT 0, pk_level integer NOT NULL DEFAULT 3, pk_time integer NOT NULL DEFAULT 0,
  ctl_code smallint NOT NULL DEFAULT 0,
  inventory   bytea NOT NULL CHECK (length(inventory) = 1728),
  magic_list  bytea NOT NULL CHECK (length(magic_list) = 180),
  effect_list bytea NOT NULL CHECK (length(effect_list) = 208),
  quest       bytea NOT NULL CHECK (length(quest) = 50),
  chat_limit_time  integer NOT NULL DEFAULT 0,
  fruit_add_points integer NOT NULL DEFAULT 0,
  fruit_sub_points integer NOT NULL DEFAULT 0,
  married          boolean NOT NULL DEFAULT false,
  marry_couple     varchar(10),
  resets           integer NOT NULL DEFAULT 0,
  master_resets    integer NOT NULL DEFAULT 0,
  is_new           boolean NOT NULL DEFAULT true,
  created_at       timestamptz NOT NULL DEFAULT now(),
  last_login_at    timestamptz,
  UNIQUE (account_id, slot)
);

CREATE TABLE warehouses (
  account_id bigint NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  number     smallint NOT NULL DEFAULT 0,
  items      bytea NOT NULL CHECK (length(items) = 1920),
  money      bigint NOT NULL DEFAULT 0,
  password   integer NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, number)
);

CREATE SEQUENCE item_serial_seq;

CREATE TABLE character_options (
  character_id bigint PRIMARY KEY REFERENCES characters(id) ON DELETE CASCADE,
  skill_keys  bytea NOT NULL,
  game_option smallint NOT NULL DEFAULT 0,
  q_key smallint NOT NULL DEFAULT 0, w_key smallint NOT NULL DEFAULT 0, e_key smallint NOT NULL DEFAULT 0,
  chat_window smallint NOT NULL DEFAULT 0
);

CREATE TABLE guilds (
  id        bigserial PRIMARY KEY,
  name      citext UNIQUE NOT NULL CHECK (length(name) BETWEEN 1 AND 8),
  mark      bytea NOT NULL CHECK (length(mark) = 32),
  score     integer NOT NULL DEFAULT 0,
  master    citext NOT NULL,
  notice    varchar(60) NOT NULL DEFAULT '',
  type      integer NOT NULL DEFAULT 0,
  rival     integer NOT NULL DEFAULT 0,
  union_id  integer NOT NULL DEFAULT 0
);

CREATE TABLE guild_members (
  name     citext PRIMARY KEY,
  guild_id bigint NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
  level    smallint NOT NULL DEFAULT 1,
  status   smallint NOT NULL DEFAULT 0
);
