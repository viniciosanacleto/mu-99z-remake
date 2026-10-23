-- Friends and mail (T_CGuid, T_FriendMain, T_FriendList, T_WaitFriend,
-- T_FriendMail). Rows follow the character by name and disappear with it
-- (WZ_DeleteCharacter deletes them explicitly in the original).
CREATE TABLE friend_main (
  guid         serial PRIMARY KEY,
  name         citext UNIQUE NOT NULL REFERENCES characters(name) ON DELETE CASCADE ON UPDATE CASCADE,
  friend_count integer NOT NULL DEFAULT 1,
  memo_count   integer NOT NULL DEFAULT 10,
  memo_total   integer NOT NULL DEFAULT 0
);

CREATE TABLE friend_list (
  guid        integer NOT NULL REFERENCES friend_main(guid) ON DELETE CASCADE,
  friend_guid integer NOT NULL REFERENCES friend_main(guid) ON DELETE CASCADE,
  friend_name citext NOT NULL,
  del         smallint NOT NULL DEFAULT 0,
  PRIMARY KEY (guid, friend_guid)
);

CREATE TABLE wait_friend (
  guid        integer NOT NULL REFERENCES friend_main(guid) ON DELETE CASCADE,
  friend_guid integer NOT NULL REFERENCES friend_main(guid) ON DELETE CASCADE,
  friend_name citext NOT NULL,
  PRIMARY KEY (guid, friend_guid)
);

CREATE TABLE friend_mail (
  memo_index  integer NOT NULL,
  guid        integer NOT NULL REFERENCES friend_main(guid) ON DELETE CASCADE,
  friend_name varchar(10) NOT NULL,
  w_date      timestamptz NOT NULL DEFAULT now(),
  subject     varchar(32) NOT NULL DEFAULT '',
  b_read      smallint NOT NULL DEFAULT 0,
  memo        bytea NOT NULL DEFAULT ''::bytea,
  photo       bytea NOT NULL DEFAULT ''::bytea,
  dir         smallint NOT NULL DEFAULT 0,
  act         smallint NOT NULL DEFAULT 0,
  PRIMARY KEY (guid, memo_index)
);
