// Package workflowports holds the contract suite of the status workflow
// repository port (app/workflow.Repository). The same cases run against the
// in-memory fake here and against the Postgres repository in
// test/integration/postgres, so a fake that drifts from the real adapter
// fails (backend/CLAUDE.md, Testing).
package workflowports
