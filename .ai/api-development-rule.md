# API Development Rules

All future control-plane API work follows these rules:

1. Start with an OpenAPI contract change and review the request/response model.
2. Keep database models, API DTOs, request types, and response types separate.
3. Generate the Go server contract and the Vue TypeScript client from the
   reviewed specification.
4. Implement business behavior behind service and repository interfaces; HTTP
   handlers only adapt protocol input/output.
5. Update the API Explorer and served OpenAPI document with every contract
   change.
6. Keep gateway streaming contracts in the protobuf definition and link them
   from the internal contract index; do not invent public APIs for member
   operator/webhook components.
7. Add contract, handler, service, and frontend tests for new operations.
8. Run formatting, generation, lint, tests, and builds before committing.
