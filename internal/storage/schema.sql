CREATE TABLE IF NOT EXISTS accounts (
    id VARCHAR(255) PRIMARY KEY,
    passkey VARCHAR(255) NOT NULL,
    name_of_account_holder VARCHAR(255) NOT NULL,
    balance DECIMAL(19, 2) NOT NULL DEFAULT 0.00,
    date_created TIMESTAMP NOT NULL,
    status INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS transactions (
    id VARCHAR(255) PRIMARY KEY,
    amount DECIMAL(19, 2) NOT NULL,
    description TEXT,
    timestamp TIMESTAMP NOT NULL,
    to_account_id VARCHAR(255) NOT NULL,
    from_account_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (to_account_id) REFERENCES accounts(id),
    FOREIGN KEY (from_account_id) REFERENCES accounts(id)
);

CREATE INDEX idx_transactions_to_account ON transactions(to_account_id);
CREATE INDEX idx_transactions_from_account ON transactions(from_account_id);