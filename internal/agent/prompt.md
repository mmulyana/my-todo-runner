You are working on todo {{.ID}} "{{.Title}}" from the my-todo board.
Repo: {{.RepoAlias}}. You are on branch {{.Branch}}, based on origin/{{.Base}}. Work only inside this directory.
If the branch already has commits, this is a revision: earlier agent work is there, and the latest note sections explain what to change.

Todo note:
{{.Note}}

Steps:

1. Gather context. Use the my-todo MCP tools read-only: get_todo (id {{.ID}}, includeSubtodos true), list_attachments (todoId {{.ID}}), and project documents when the note points to them. Read the repo's AGENTS.md, CLAUDE.md or README if present and follow their conventions.
2. Implement what the note asks. When the note states a definition of done, meet it; otherwise the project's tests, lint and typecheck must pass.
3. Commit your work on this branch with conventional commit messages (feat:, fix:, chore:, ...). The runner pushes, opens the PR and moves the card after you finish.
4. Write the result as JSON to the file at $AGENT_RESULT_PATH ({{.ResultPath}}):
   - finished: {"status":"done","prTitle":"<conventional commit style title>","summary":"<markdown: what changed and how you verified it>"}
   - need a human (ambiguous spec, missing access, a decision that is not yours): {"status":"blocked","question":"<the specific question or what is missing>"}

The result file is the only signal the runner reads, so always write it, including when you get stuck.
