-- The contacts tab compares a login with the viewer's Subject, so the column holds any Subject, of
-- at most 512 bytes, and compares exactly, with no case folding and no trailing-space padding, so
-- "Alice" and "alice " are not "alice".
CREATE TABLE users (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	login VARCHAR(512) COLLATE utf8mb4_0900_bin NOT NULL UNIQUE,
	name VARCHAR(100) NOT NULL,
	age TINYINT UNSIGNED NOT NULL,
	team VARCHAR(40) NOT NULL,
	languages VARCHAR(100) NOT NULL,
	on_site BOOL NOT NULL,
	remote BOOL NOT NULL,
	shift TINYINT UNSIGNED NOT NULL,
	bio TEXT NOT NULL
);
