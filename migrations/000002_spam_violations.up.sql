CREATE TABLE IF NOT EXISTS spam_violations (
   id SERIAL PRIMARY KEY,
   user_id BIGINT NOT NULL,
   group_id BIGINT NOT NULL,
   topic_id INT,
   message_text TEXT,
   violation_type VARCHAR(50) NOT NULL,
   match_found VARCHAR(255),
   created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_spam_violations_user ON spam_violations(user_id);
CREATE INDEX idx_spam_violations_created ON spam_violations(created_at);
