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
|  | Review the tests in @backend/rest/upload_test.go for thoroughness and correctness. | GLM 5.3 |  |
|  | Add a test to cover multiple files uploaded to a single DB, and add assertions to ensure events are related to the uploaded files. | Deepseek v4.1 Flash |  |
|  | Add tests to ensure response code strings such as "200 - OK" are properly stripped before being parsed as an int. | Deepseek v4.1 Flash |  |
|  | Update the test cases to reflect the design change in the UploadFIle endpoint.  Event field parsing is now done out-of-band. | Deepseek v4.1 Flash |  |
| Files get/delete implementaiton | Implement the get and delete handlers for for files in @backend/rest/register.go .  Ignore analysis, errors, and events for now. | Deepseek v4.1 Flash | Ollama Cloud |
| Event parse worker implementation | Finish implementing the event parser in @backend/worker/parser.go .  It needs to query files with FilesStatusUploaded, then query batches of events for the each file, parse them, and persist their new state.  Once all of the events for a file are parse, the file's status should be updated. | Deepseek v4.1 Flash | Ollama Cloud |
