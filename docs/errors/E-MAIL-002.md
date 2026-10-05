# E-MAIL-002: invalid address or no recipients

Addresses must be plain ASCII addresses such as `name@company.example`, at most 254 bytes, with no display name, angle brackets, square brackets or line breaks inside the address. The domain must be one the permission list could name in `email.to_domains`: letters, digits and hyphens in dot-separated parts, ending in a part of letters. IP addresses and international domain names are refused. A message needs at least one recipient.

**Fix:** write plain addresses and put the person's name in `Name`.
