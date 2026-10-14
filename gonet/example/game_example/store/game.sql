-- 游戏库表结构。由 store 在 Open 时执行（IF NOT EXISTS）。
CREATE TABLE IF NOT EXISTS role (
  roleid     VARCHAR(64)  NOT NULL,
  level      INT          NOT NULL,
  name       VARCHAR(64)  NOT NULL,
  gender         TINYINT      NOT NULL,
  lastlogintime  BIGINT       NOT NULL DEFAULT 0,
  lastlogouttime BIGINT       NOT NULL DEFAULT 0,
  updated_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (roleid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS friend (
  roleid   VARCHAR(64) NOT NULL,
  friendid VARCHAR(64) NOT NULL,
  PRIMARY KEY (roleid, friendid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS friend_request (
  fromid VARCHAR(64) NOT NULL,
  toid   VARCHAR(64) NOT NULL,
  time   BIGINT      NOT NULL,
  PRIMARY KEY (fromid, toid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS guild (
  id      VARCHAR(64)  NOT NULL,
  name    VARCHAR(64)  NOT NULL,
  leader  VARCHAR(64)  NOT NULL,
  notice  VARCHAR(255) NOT NULL DEFAULT '',
  created BIGINT       NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_guild_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS guild_member (
  roleid  VARCHAR(64) NOT NULL,
  guildid VARCHAR(64) NOT NULL,
  `rank`  TINYINT     NOT NULL,
  time    BIGINT      NOT NULL,
  PRIMARY KEY (roleid),
  KEY idx_guild_member_guild (guildid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS guild_apply (
  guildid VARCHAR(64) NOT NULL,
  roleid  VARCHAR(64) NOT NULL,
  time    BIGINT      NOT NULL,
  PRIMARY KEY (guildid, roleid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_mission (
  roleid VARCHAR(64)  NOT NULL,
  data   JSON         NOT NULL,
  time   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (roleid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
