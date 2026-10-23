-- ReconnectData (CReconnect of the WizardTeam GameServer, DataServer
-- C0 00 / C0 01 / replay on GameServer connect): the reconnect record of
-- a character that left a game server, kept ReconnectTime seconds. Data is
-- the 400-byte block the GameServer sends (the 0x13C-byte record first).
CREATE TABLE reconnect_data (
  name        citext  NOT NULL,
  server_code integer NOT NULL,
  data        bytea   NOT NULL,
  time        integer NOT NULL,
  PRIMARY KEY (name, server_code)
);
