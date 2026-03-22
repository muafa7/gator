# Gator 🐊

Gator is a simple CLI RSS feed aggregator written in Go.
You can add feeds, follow them, periodically fetch posts, and browse everything from your terminal.

---

## What you need

Before running this, make sure you have:

* Go installed
* PostgreSQL installed and running

---

## Install the CLI

```bash
go install github.com/muafa7/gator@latest
```

Make sure your Go bin directory is in your PATH:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

After that, you should be able to run:

```bash
gator
```

---

## Database setup

Create a database:

```sql
CREATE DATABASE gator;
```

Then run your migrations so all tables are created.

---

## Config

Create this file:

```bash
~/.gatorconfig.json
```

Example:

```json
{
  "db_url": "postgres://USERNAME:PASSWORD@localhost:5432/gator?sslmode=disable",
  "current_user_name": ""
}
```

---

## How to use

During development you can use:

```bash
go run . register muafa
```

But normally you should use the installed CLI:

```bash
gator register muafa
```

---

## Basic commands

Register & login:

```bash
gator register muafa
gator login muafa
```

Add a feed:

```bash
gator addfeed bootdev https://www.wagslane.dev/index.xml
```

Follow / unfollow:

```bash
gator follow https://www.wagslane.dev/index.xml
gator unfollow https://www.wagslane.dev/index.xml
```

See feeds:

```bash
gator feeds
gator following
```

Start fetching posts:

```bash
gator agg 30s
```

Browse saved posts:

```bash
gator browse
gator browse 5
```

---

## Notes

* `go run .` is just for development
* `gator` is the real CLI you’ll use after install
* posts are stored in your database
* duplicate posts are ignored (based on URL)
* the aggregator runs forever until you stop it

---

## Repo

https://github.com/muafa7/gator
