# LLM Use

Prompts and commands used during development of this app.

The harness used for all interactions was Kilo Code.

| Session | Prompt | Model | Provider |
| -- | -- | -- | -- | -- |
| DB setup | Finish implementing the DB setup function in @db.go. | Deepseek v4.1 Flash | Ollama Cloud |
| Health check setup | Implement the /healthz and /readyz handlers in @rest. | Deepseek v4.1 Flash | Ollama Cloud |
|  | Use the testify assert and require packages in the test code. |  |  |
|  | The SQLite database setup helper for tests should be provided by the db package. |  |  |