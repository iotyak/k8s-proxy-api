# Future Suggestions

This file captures ideas, improvements, and technical debt that are worth considering but not urgent enough to implement immediately.

## Namespace Listing Endpoints (Task 13)

### Documentation Improvement: Error Responses
- The current documentation for `/deployments` and `/pods` only covers success responses.
- Add an "Error Responses" section (similar to the Delete endpoint) covering:
  - `405 Method Not Allowed`
  - `400 Bad Request` (malformed path)
  - `503 Service Unavailable` (Kubernetes client issues)

This should be added to both `README.MD` and `specs.md` for consistency.