# Triage Labels

Issue labels live in GitHub. Always read the current list with `gh label list`
before doing label-sensitive work.

## Current Labels

| Label | Meaning | Agent use |
| ----- | ------- | --------- |
| `bug` | Something is not working | Agents may apply when the issue describes a defect. |
| `documentation` | Documentation improvement | Agents may apply for docs-only work. |
| `duplicate` | Existing issue already covers this | Agents may suggest or apply only when the duplicate issue is clearly identified. |
| `enhancement` | Existing behavior, tests, or implementation can be improved | Agents may apply for non-bug improvements. |
| `good first issue` | Good for newcomers | Agents may suggest; humans decide final suitability. |
| `help wanted` | Extra attention is needed | Agents may suggest; humans decide final suitability. |
| `invalid` | The issue is not valid as filed | Agents may suggest; humans should make final closure decisions. |
| `question` | Further information is requested | Agents may apply when asking the reporter for more information. |
| `security` | Security problem | Agents may apply for security-relevant issues, but do not disclose unvalidated exploit details beyond what is needed for triage. |
| `wontfix` | This will not be worked on | Human decision label; agents may recommend but should not apply without explicit human approval. |

## Human-Only Decision Labels

The following labels are operator decision states. Agents must not apply, remove,
or change these labels unless the user explicitly asks for that exact label
operation. Agents may mention that one of these labels seems appropriate in a
comment or report.

| Label | Meaning |
| ----- | ------- |
| `accepted` | The claims are accepted and approved for action. |
| `needs-explain` | The human operator does not understand the claim or is not convinced yet. |
| `rejected` | The claims are explicitly rejected. |

## Role Mapping

Some skills use generic role names. Map them to this repo's labels as follows:

| Generic role | Use in this repo |
| ------------ | ---------------- |
| `needs-triage` | Do not apply a label automatically; leave unlabeled or use a topical label such as `bug`, `enhancement`, `documentation`, or `security`. |
| `needs-info` | Use `question` when asking for more information. |
| `ready-for-agent` | No direct label. Mention readiness in the issue/comment instead. |
| `ready-for-human` | No direct label. Mention that human judgment is needed instead. |
| `wontfix` | Use `wontfix` only with explicit human approval. |
