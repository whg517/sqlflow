-- e2e/seed/init.sql
-- MySQL test database initialization for E2E tests
--
-- This script runs when the mysql-test container starts for the first time.
-- It creates test tables and seed data that real E2E tests depend on.
--
-- Note: Application users (admin/developer/dba) are stored in SQLite (managed by Go app),
-- NOT in MySQL. This file only creates MySQL-side test data (tables that E2E queries target).

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- ============================================================
-- sys_user: Main test table for query E2E tests
-- ============================================================
DROP TABLE IF EXISTS `sys_user`;
CREATE TABLE `sys_user` (
    `id`          BIGINT       NOT NULL AUTO_INCREMENT,
    `username`    VARCHAR(64)  NOT NULL,
    `password`    VARCHAR(255) NOT NULL,
    `email`       VARCHAR(128) DEFAULT NULL,
    `phone`       VARCHAR(32)  DEFAULT NULL,
    `role`        VARCHAR(32)  NOT NULL DEFAULT 'developer',
    `status`      TINYINT      NOT NULL DEFAULT 1 COMMENT '1=active, 0=disabled',
    `created_at`  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_username` (`username`),
    KEY `idx_role` (`role`),
    KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Seed data for sys_user
INSERT INTO `sys_user` (`username`, `password`, `email`, `phone`, `role`, `status`) VALUES
    ('alice',  'hashed_password_alice_12345',  'alice@example.com',   '13800000001', 'admin',     1),
    ('bob',    'hashed_password_bob_67890',    'bob@example.com',     '13800000002', 'developer', 1),
    ('charlie','hashed_password_charlie_11111', 'charlie@example.com', '13800000003', 'dba',       1),
    ('diana',  'hashed_password_diana_22222',  'diana@example.com',   '13800000004', 'developer', 1),
    ('eve',    'hashed_password_eve_33333',    'eve@example.com',     '13800000005', 'developer', 0);

-- ============================================================
-- orders: Secondary test table for JOIN queries
-- ============================================================
DROP TABLE IF EXISTS `orders`;
CREATE TABLE `orders` (
    `id`           BIGINT        NOT NULL AUTO_INCREMENT,
    `user_id`      BIGINT        NOT NULL,
    `product_name` VARCHAR(128)  NOT NULL,
    `quantity`     INT           NOT NULL DEFAULT 1,
    `price`        DECIMAL(10,2) NOT NULL,
    `status`       VARCHAR(32)   NOT NULL DEFAULT 'pending' COMMENT 'pending/paid/shipped/completed/cancelled',
    `created_at`   DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_status` (`status`),
    CONSTRAINT `fk_orders_user` FOREIGN KEY (`user_id`) REFERENCES `sys_user` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Seed data for orders
INSERT INTO `orders` (`user_id`, `product_name`, `quantity`, `price`, `status`) VALUES
    (1, 'SQLFlow Enterprise License',  1, 9999.00, 'completed'),
    (2, 'PostgreSQL Monitoring Addon',  2, 299.00,  'paid'),
    (2, 'Query Audit Extension',        1, 499.00,  'shipped'),
    (3, 'MySQL Performance Toolkit',   1, 799.00,  'pending'),
    (4, 'Data Masking Module',          3, 199.00,  'completed'),
    (5, 'Backup Scheduler Plugin',      1, 399.00,  'cancelled');

-- ============================================================
-- products: Reference table for query tests
-- ============================================================
DROP TABLE IF EXISTS `products`;
CREATE TABLE `products` (
    `id`          BIGINT        NOT NULL AUTO_INCREMENT,
    `name`        VARCHAR(128)  NOT NULL,
    `category`    VARCHAR(64)   NOT NULL DEFAULT 'general',
    `price`       DECIMAL(10,2) NOT NULL,
    `stock`       INT           NOT NULL DEFAULT 0,
    `description` TEXT          DEFAULT NULL,
    `created_at`  DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_name` (`name`),
    KEY `idx_category` (`category`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Seed data for products
INSERT INTO `products` (`name`, `category`, `price`, `stock`, `description`) VALUES
    ('SQLFlow Enterprise License',  'license',  9999.00, 999, 'Full enterprise SQL management platform'),
    ('PostgreSQL Monitoring Addon', 'addon',     299.00, 500, 'Real-time PostgreSQL monitoring'),
    ('Query Audit Extension',       'addon',     499.00, 300, 'Advanced SQL audit and compliance'),
    ('MySQL Performance Toolkit',   'toolkit',   799.00, 200, 'MySQL performance diagnostics'),
    ('Data Masking Module',         'security',  199.00, 800, 'Automatic data masking for PII'),
    ('Backup Scheduler Plugin',     'utility',   399.00, 150, 'Automated backup scheduling');

SET FOREIGN_KEY_CHECKS = 1;
