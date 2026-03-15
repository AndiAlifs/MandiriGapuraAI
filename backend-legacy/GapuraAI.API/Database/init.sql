-- =============================================================================
-- GAPURA AI Studio — MySQL Initialization Script
-- Mission 1: Foundation Layer
-- Target: MySQL 8.0
-- =============================================================================

CREATE DATABASE IF NOT EXISTS `gapura_ai_studio`
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;

USE `gapura_ai_studio`;

-- -----------------------------------------------------------------------------
-- Table: Apps_Auth
-- Stores registered internal applications and their credentials/quotas.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `Apps_Auth` (
    `AppID`           INT             NOT NULL AUTO_INCREMENT,
    `ProjectName`     VARCHAR(100)    NOT NULL,
    `Username`        VARCHAR(50)     NOT NULL,
    `PasswordHash`    VARCHAR(255)    NOT NULL,
    `DailyTokenLimit` INT             NOT NULL DEFAULT 100000,
    PRIMARY KEY (`AppID`),
    UNIQUE KEY `UQ_Apps_Auth_Username` (`Username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- -----------------------------------------------------------------------------
-- Table: Model_Registry
-- Catalog of all available AI models (cloud + local fallback).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `Model_Registry` (
    `ModelID`         INT             NOT NULL AUTO_INCREMENT,
    `ModelName`       VARCHAR(100)    NOT NULL,
    `Provider`        VARCHAR(50)     NOT NULL,
    `CostPer1kInput`  DECIMAL(10,6)   NOT NULL DEFAULT 0.000000,
    `CostPer1kOutput` DECIMAL(10,6)   NOT NULL DEFAULT 0.000000,
    `IsLocalFallback` BOOLEAN         NOT NULL DEFAULT FALSE,
    PRIMARY KEY (`ModelID`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- -----------------------------------------------------------------------------
-- Table: Audit_Logs
-- Immutable audit trail of every request processed by the GAPURA pipeline.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `Audit_Logs` (
    `LogID`           BIGINT          NOT NULL AUTO_INCREMENT,
    `AppID`           INT             NOT NULL,
    `ModelUsed`       VARCHAR(100)    NOT NULL,
    `OriginalPrompt`  TEXT            NOT NULL,
    `ScrubbedPrompt`  TEXT            NOT NULL,
    `ResponseText`    TEXT            NULL,
    `InputTokens`     INT             NOT NULL DEFAULT 0,
    `OutputTokens`    INT             NOT NULL DEFAULT 0,
    `CalculatedCost`  DECIMAL(10,6)   NOT NULL DEFAULT 0.000000,
    `LatencyMS`       INT             NOT NULL DEFAULT 0,
    `Timestamp`       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`LogID`),
    CONSTRAINT `FK_AuditLogs_AppsAuth` FOREIGN KEY (`AppID`)
        REFERENCES `Apps_Auth` (`AppID`)
        ON DELETE RESTRICT
        ON UPDATE CASCADE,
    INDEX `IX_AuditLogs_AppID` (`AppID`),
    INDEX `IX_AuditLogs_Timestamp` (`Timestamp` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =============================================================================
-- SEED DATA
-- =============================================================================

-- Seed: One dummy application (Project: PopCorn RAG)
-- PasswordHash is a BCrypt placeholder for the password 'P@ssw0rd!' 
INSERT INTO `Apps_Auth` (`ProjectName`, `Username`, `PasswordHash`, `DailyTokenLimit`)
VALUES ('PopCorn RAG', 'popcorn_rag', '$2a$12$LJ3m4ys3Sz8MfVfCqOaO..exampleHashPlaceholder00000000000', 100000);

-- Seed: Cloud model — gpt-4o-mini (OpenAI)
INSERT INTO `Model_Registry` (`ModelName`, `Provider`, `CostPer1kInput`, `CostPer1kOutput`, `IsLocalFallback`)
VALUES ('gpt-4o-mini', 'OpenAI', 0.000150, 0.000600, FALSE);

-- Seed: Local Fallback model — llama3-8b-local (Ollama on VPS)
INSERT INTO `Model_Registry` (`ModelName`, `Provider`, `CostPer1kInput`, `CostPer1kOutput`, `IsLocalFallback`)
VALUES ('llama3-8b-local', 'Ollama', 0.000000, 0.000000, TRUE);
