-- An owner compares exactly, with no case folding and no trailing-space padding.
CREATE TABLE notes (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	owner VARCHAR(512) COLLATE utf8mb4_0900_bin NOT NULL,
	title VARCHAR(100) NOT NULL,
	body TEXT NOT NULL,
	KEY (owner)
);
