# E-MAIL-004: header, subject, name or attachment field refused

A header field of the message has a line break or another control character, which could add hidden recipients or headers, is not valid UTF-8, or is too long; or the name of the sender, a recipient or Reply-To holds characters that reverse how text is shown; or the message sets a header that is not allowed, or whose name is not plain ASCII; or an attachment has a name that is a path, `.` or `..`, or holds control characters or characters that reverse how text is shown; or its content type is not a valid media type of at most 255 ASCII characters. Messages may set only In-Reply-To, References, List-Id, List-Unsubscribe, List-Unsubscribe-Post, Auto-Submitted and Precedence.

**Fix:** remove line breaks and control characters, and set only the allowed headers.
