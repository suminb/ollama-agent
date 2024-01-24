Requires the following environment variables:

- `GITHUB_APP_ID` (e.g., 750)
- `GITHUB_INSTALLATION_ID` (e.g, 5182)
- `GITHUB_PRIVATE_KEY_FILE` (e.g., `ollama-reviewer.private-key.pem`)

Some optional environment variables may be set:

- `REDIS_HOST`, if Redis is used as a message queue
- `AWS_S3_ENDPOINT`, if non-AWS S3 storage is used
- `GITHUB_BASE_URL` (e.g., `https://yourcompany.com/api/v3`)