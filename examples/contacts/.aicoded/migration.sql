-- A login is compared with the viewer's Subject, so the column holds any Subject, of at most 512
-- bytes, and compares exactly, with no case folding and no trailing-space padding, so "Alice"
-- and "alice " are not "alice".
CREATE TABLE contacts (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	login VARCHAR(512) COLLATE utf8mb4_0900_bin NOT NULL,
	kind ENUM('phone', 'email') NOT NULL,
	value VARCHAR(254) NOT NULL,
	UNIQUE KEY (login, kind, value)
);
