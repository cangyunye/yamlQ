-- yamlq Oracle E2E 测试表（在 APPUSER schema 下执行, 幂等可重复）
-- 对齐 mysql/postgres 的 e2e_users, 供跨库用例 (testdata/e2e_multi_db.yaml oracle_users) 使用。
-- 手动执行：
--   docker exec -i oracle sqlplus -s appuser/App123!@//localhost:1521/XEPDB1 < oracle/seed_appuser.sql

SET DEFINE OFF
WHENEVER SQLERROR CONTINUE

BEGIN
  EXECUTE IMMEDIATE 'DROP TABLE e2e_users';
EXCEPTION WHEN OTHERS THEN NULL;
END;
/

CREATE TABLE e2e_users (
  id         NUMBER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name       VARCHAR2(50),
  email      VARCHAR2(100),
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO e2e_users (name, email) VALUES ('Alice', 'alice@test.com');
INSERT INTO e2e_users (name, email) VALUES ('Bob', 'bob@test.com');
INSERT INTO e2e_users (name, email) VALUES ('Charlie', 'charlie@test.com');

COMMIT;
EXIT;
