<!-- omit in toc -->
# Contributing to RLark

Thanks for taking the time to contribute to RLark!

All types of contributions are encouraged and valued. Please make sure to read the relevant section before making your contribution. The community looks forward to your contributions.

> And if you like the project, but just don't have time to contribute, that's fine. There are other easy ways to support the project:
> - Star the project
> - Tweet about it
> - Refer this project in your project's readme
> - Mention the project at local meetups and tell your friends/colleagues

<!-- omit in toc -->
## Table of Contents

- [Contribution Procedure](#contribution-procedure)
- [Pull Request Guidelines](#pull-request-guidelines)
  - [Code Style and Formatting](#code-style-and-formatting)
  - [Commit Messages and Signed-off-by](#commit-messages-and-signed-off-by)
  - [PR Title and Description](#pr-title-and-description)
  - [Review Process](#review-process)

## Contribution Procedure

All contributions (including the project team's) take the form of [GitHub Pull Requests](https://github.com/RLinf/RLark/pulls).
To contribute, first [fork the repository](https://github.com/RLinf/RLark/fork) and clone it to your local machine.
Then, create a new development branch from `main` for your contribution:

```bash
git checkout main
git pull origin main
git checkout -b feature/your-feature-name
```

Make sure you read and follow the [Pull Request Guidelines](#pull-request-guidelines) below before committing and pushing your changes.

Push your changes to your forked repository:

```bash
git push origin feature/your-feature-name
```

Then, open a [Pull Request](https://github.com/RLinf/RLark/compare) against the `main` branch of the original repository.
We will review your changes and run CI tests before merging them.

## Pull Request Guidelines

### THE PRIME DIRECTIVE

**All user-facing changes must be accompanied by tests and documentation, which must be followed and validated by at least one reviewer to ensure its correctness.**

### Code Style and Formatting

* **Go** (control plane & agents): Follow [Effective Go](https://go.dev/doc/effective_go) and standard Go conventions.

  Run linting before committing:
  ```bash
  make lint-go && make lint-web
  ```

* **TypeScript/React** (web UI): Follow standard React/TypeScript conventions.

  Run linting before committing:
  ```bash
  make -C apps/rlark-ui lint
  ```

* **Comments & Documentation**: All code should include sufficient comments to ensure future contributors can easily understand the code. Public functions and types should have doc comments.

* **Error Handling**: All errors should be handled explicitly. Error messages should be clear and meaningful. In Go, never ignore errors with `_` without a comment explaining why.

* **Logging**: Use structured logging via `apps/rlark/pkg/log`. Avoid `fmt.Println` for production code.

* **Configuration**: Configuration files (YAML) should use static values only. Do not perform calculations or set dynamic values in YAML files. All values should be treated as read-only by code.

* **Tests**: Include tests for all new features. Go tests use the standard `go test` framework. Refer to existing tests for examples.

### Commit Messages and Signed-off-by

All commits must include a `Signed-off-by:` line at the end of the commit message.
Using the `-s` flag will automatically achieve this:

```bash
git add .
git commit -s
```

You can enable automatic sign-off in your IDE. In VS Code, open the [settings editor](https://code.visualstudio.com/docs/configure/settings) and enable `Git: Always Sign Off`.

The commit message should follow the [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) standard:

```
<type>(<scope>): <description>
```

Where `<type>` commonly includes:
- `feat`: a new feature for the user
- `fix`: a bug fix for the user
- `docs`: changes to the documentation
- `style`: formatting, missing semicolons, etc; no code change
- `refactor`: refactoring production code, e.g. renaming a variable
- `test`: adding missing tests, refactoring tests; no production code change
- `chore`: updating build tasks, package manager configs, etc; no production code change

### PR Title and Description

All PR titles should follow the same format as commit messages:

```
<type>(<scope>): <description>
```

The PR description should include:
- **Description**: What changes were made and why
- **Motivation and Context**: Link to related issues if applicable
- **How has this been tested?**: Describe testing steps and results

### Review Process

* After you have submitted your PR, it will be assigned to maintainers for review.

* Reviewers will provide feedback every 1-2 business days. If the PR is not reviewed within 3 business days, please feel free to ping the maintainers in the PR thread.

* After the review, if changes are required, the contributor should address the comments and ping the reviewer to re-review the PR.

* Please respond to all comments within a reasonable time frame. If a comment isn't clear or you disagree with a suggestion, feel free to ask for clarification or discuss the suggestion.

* If you cannot respond to any comment within 7 days, the PR will be considered inactive and may be closed. You can always reopen the PR later when you are ready to address the comments.
