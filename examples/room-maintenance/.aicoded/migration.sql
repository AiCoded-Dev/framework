CREATE TABLE rooms (
	number SMALLINT UNSIGNED PRIMARY KEY,
	floor TINYINT UNSIGNED NOT NULL
);

-- An author is the viewer's Subject, of at most 512 bytes, and compares exactly, with no case
-- folding and no trailing-space padding, so "Alice" and "alice " are not "alice".
CREATE TABLE tickets (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	room SMALLINT UNSIGNED NOT NULL,
	title VARCHAR(100) NOT NULL,
	details TEXT NOT NULL,
	status ENUM('open', 'in_progress', 'done') NOT NULL,
	author VARCHAR(512) COLLATE utf8mb4_0900_bin NOT NULL,
	created DATETIME NOT NULL,
	updated DATETIME NOT NULL,
	KEY (author),
	FOREIGN KEY (room) REFERENCES rooms (number)
);
