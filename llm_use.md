# LLM Use

Prompts and commands used during development of this app.

Kilo Code was used as the harness for all interactions.

| Session | Prompt | Model | Provider |
| -- | -- | -- | -- |
| DB setup | Finish implementing the DB setup function in @db.go. | Deepseek v4.1 Flash | Ollama Cloud |
| Health check setup | Implement the /healthz and /readyz handlers in @rest. | Deepseek v4.1 Flash | Ollama Cloud |
|  | Use the testify assert and require packages in the test code. |  |  |
|  | The SQLite database setup helper for tests should be provided by the db package. |  |  |
| Upload handler initial implementation | Finish implementing the file upload handler in @backend/rest/upload.go .  The handler should accept a CSV file as multipart form data, create a file object in the database per @backend/db/file.go .  The upload handler should only add values to the Filename, Bytes, Status, and Raw fields.  Everything else should be kept at zero-value for out-of-band processing. | Deepseek v4.1 Flash | Ollama Cloud |
| UploadFile endpoint test creation | Create tests for the UploadFIle endpoint in @backend/rest/upload.go .  Cover the inclusion and omission of every field in the File model, as well as valid and invald values where applicable. | Deepseek v4.1 Flash | Ollama Cloud |
