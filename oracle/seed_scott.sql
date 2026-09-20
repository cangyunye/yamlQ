-- yamlq Oracle 演示/测试种子（在 SCOTT schema 下执行，幂等可重复）
-- 手动执行：
--   docker exec -i oracle sqlplus -s scott/tiger@//localhost:1521/XEPDB1 < oracle/seed_scott.sql
--
-- 表：t_product / t_order / t_task（含 FK、日期、长文本、枚举字段）
-- 视图：v_high_priority
-- 用于: 终端表格展示（宽度/主题）、YAML enum_map 枚举翻译、money/日期 converter

SET DEFINE OFF
WHENEVER SQLERROR CONTINUE

BEGIN
  EXECUTE IMMEDIATE 'DROP TABLE t_order';
EXCEPTION WHEN OTHERS THEN NULL;
END;
/
BEGIN
  EXECUTE IMMEDIATE 'DROP TABLE t_task';
EXCEPTION WHEN OTHERS THEN NULL;
END;
/
BEGIN
  EXECUTE IMMEDIATE 'DROP TABLE t_product';
EXCEPTION WHEN OTHERS THEN NULL;
END;
/
BEGIN
  EXECUTE IMMEDIATE 'DROP VIEW v_high_priority';
EXCEPTION WHEN OTHERS THEN NULL;
END;
/

-- ── 商品表（category/active 为枚举字段, remark 为超宽文本, 演示宽度折行）──
CREATE TABLE t_product (
  product_id NUMBER PRIMARY KEY,
  name       VARCHAR2(40),
  category   VARCHAR2(20),
  price      NUMBER(10,2),
  stock      NUMBER(8),
  active     NUMBER(1),
  created_at DATE,
  remark     VARCHAR2(200)
);

INSERT INTO t_product VALUES (1, '机械键盘 K87', 'PERI', 329.00, 120, 1,
  TO_DATE('2024-02-11','YYYY-MM-DD'),
  '87键 红轴 无线双模 铝合金外壳 支持三设备切换 Type-C 快充');
INSERT INTO t_product VALUES (2, '27寸 4K 显示器', 'PC', 1899.00, 45, 1,
  TO_DATE('2024-03-02','YYYY-MM-DD'),
  'IPS 面板 3840x2160 HDR400 Type-C 90W 反向供电 出厂校色 ΔE<2');
INSERT INTO t_product VALUES (3, '降噪耳机 WH-1000', 'AUDIO', 1099.00, 80, 0,
  TO_DATE('2024-05-18','YYYY-MM-DD'),
  '无线蓝牙 5.3 自适应降噪 40h 续航 多点连接 支持 LDAC 高清音频编解码');
INSERT INTO t_product VALUES (4, 'USB-C 扩展坞', 'PERI', 259.00, 0, 1,
  TO_DATE('2024-06-30','YYYY-MM-DD'),
  '七合一 支持 HDMI 4K60 千兆网口 SD/TF 读卡 100W PD 供电 铝合金一体成型');
INSERT INTO t_product VALUES (5, '智能音箱 mini', 'AUDIO', 199.00, 200, 0,
  TO_DATE('2024-08-22','YYYY-MM-DD'),
  '语音助手 蓝牙 5.0 5W 全频扬声器 支持双机立体声组网 音乐闹钟');
INSERT INTO t_product VALUES (6, '护眼台灯', 'LIFE', 149.00, 66, 1,
  TO_DATE('2024-10-09','YYYY-MM-DD'),
  '国AA级照度 无频闪 RG0 防蓝光 触控调光 45min 定时休息 色温三档');

-- ── 订单表（status 为枚举字段；FK 关联商品）──
CREATE TABLE t_order (
  order_id   NUMBER PRIMARY KEY,
  order_no   VARCHAR2(16) UNIQUE,
  customer   VARCHAR2(30),
  product_id NUMBER REFERENCES t_product(product_id),
  qty        NUMBER(6),
  amount     NUMBER(10,2),
  status     VARCHAR2(12),
  created_at TIMESTAMP
);

INSERT INTO t_order VALUES (1, 'ORD-1001', '林小满', 1, 2, 658.00, 'PAID',
  TIMESTAMP '2025-01-06 10:12:33');
INSERT INTO t_order VALUES (2, 'ORD-1002', '周舟', 2, 1, 1899.00, 'SHIPPED',
  TIMESTAMP '2025-01-18 09:05:12');
INSERT INTO t_order VALUES (3, 'ORD-1003', '陈默', 3, 1, 1099.00, 'PENDING',
  TIMESTAMP '2025-02-03 14:22:41');
INSERT INTO t_order VALUES (4, 'ORD-1004', '林小满', 4, 3, 777.00, 'COMPLETED',
  TIMESTAMP '2025-02-20 08:30:00');
INSERT INTO t_order VALUES (5, 'ORD-1005', '赵磊', 5, 2, 398.00, 'CANCELLED',
  TIMESTAMP '2025-03-11 16:45:22');
INSERT INTO t_order VALUES (6, 'ORD-1006', '周舟', 6, 5, 745.00, 'PAID',
  TIMESTAMP '2025-04-02 11:08:55');
INSERT INTO t_order VALUES (7, 'ORD-1007', '孙悦', 1, 1, 329.00, 'COMPLETED',
  TIMESTAMP '2025-04-28 19:33:47');
INSERT INTO t_order VALUES (8, 'ORD-1008', '王芳', 2, 1, 1899.00, 'PAID',
  TIMESTAMP '2025-05-15 10:00:09');
INSERT INTO t_order VALUES (9, 'ORD-1009', '郑凯', 3, 1, 1099.00, 'SHIPPED',
  TIMESTAMP '2025-06-01 12:18:33');

-- ── 任务表（priority/state 两个枚举字段 + 日期, 宽标题演示宽度控制）──
CREATE TABLE t_task (
  task_id    NUMBER PRIMARY KEY,
  title      VARCHAR2(120),
  owner      VARCHAR2(30),
  priority   VARCHAR2(8),
  state      VARCHAR2(12),
  due        DATE,
  created_at TIMESTAMP
);

INSERT INTO t_task VALUES (1, '升级网关日志组件', '阿伟', 'HIGH', 'DONE',
  TO_DATE('2025-06-30','YYYY-MM-DD'), TIMESTAMP '2025-05-10 09:00:00');
INSERT INTO t_task VALUES (2, 'Oracle 驱动压测', '阿珍', 'HIGH', 'DOING',
  TO_DATE('2025-08-15','YYYY-MM-DD'), TIMESTAMP '2025-05-18 10:30:00');
INSERT INTO t_task VALUES (3, '补全 Oracle E2E 用例', '小明', 'MEDIUM', 'TODO',
  TO_DATE('2025-09-01','YYYY-MM-DD'), TIMESTAMP '2025-06-02 14:00:00');
INSERT INTO t_task VALUES (4, '终端渲染自适应: 表格过宽时按列宽折行, 避免截断关键信息且不撑爆终端', '大熊', 'MEDIUM', 'DOING',
  TO_DATE('2025-07-20','YYYY-MM-DD'), TIMESTAMP '2025-06-10 11:20:00');
INSERT INTO t_task VALUES (5, '主题配色评审', '小丽', 'LOW', 'TODO',
  TO_DATE('2025-10-01','YYYY-MM-DD'), TIMESTAMP '2025-06-15 16:00:00');
INSERT INTO t_task VALUES (6, '枚举映射 YAML 联调', '阿伟', 'HIGH', 'BLOCKED',
  TO_DATE('2025-08-05','YYYY-MM-DD'), TIMESTAMP '2025-06-20 09:45:00');
INSERT INTO t_task VALUES (7, '文档同步与发布', '小丽', 'LOW', 'DONE',
  TO_DATE('2025-06-10','YYYY-MM-DD'), TIMESTAMP '2025-06-01 08:00:00');

-- DB 视图示例：高优先级任务（yamlq 视图可直接查它）
CREATE OR REPLACE VIEW v_high_priority AS
  SELECT task_id, title, owner, state, due
  FROM t_task
  WHERE priority = 'HIGH';

COMMIT;
EXIT;
