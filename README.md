# 🥸 Brows

Brows is a CLI tool to browse GitHub releases

## Example:

```
> brows organization/repo 1.2.3
```

## Demo:

![brows demo gif](https://gist.githubusercontent.com/rubysolo/b950484268a607cfefaf644c3b5342da/raw/b029cbcdbcd5c2769ec79cf70bcbd4a097796da7/brows.gif)

## Installation:

```
brew tap rubysolo/tools
brew install brows
```

## Configuration:
  * (Required) A GitHub auth token is required. It can be set by the `GITHUB_OAUTH_TOKEN` environment variable to a [GitHub PAT](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/creating-a-personal-access-token) or, if the environment variable is not set, Brows will attempt to use the token from the [GitHub CLI](https://cli.github.com/) (`gh auth token`) if available (CLI installed and logged in).
  * (Optional) Create a config file at `$HOME/.config/brows.yml` and set a `default_org` key, like:

```
default_org: organization
```

## Credits:

This would not be possible without the fantastic CLI libraries from [Charm](https://charm.sh/)!
