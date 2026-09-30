# Security

chat-term is a remote shell: anyone who can message it from an allowed account
can run commands on your machine. Please treat security reports seriously and
report them privately.

## Reporting a problem

Use GitHub's private reporting: open the **Security** tab of this repository
and choose **Report a vulnerability**. Please don't open a public issue for
anything that could let someone reach the shell, read the login store, or get
around the allowlist.

Include what you did, what happened, and the version (`chat-term version`).
You should hear back within a week.

## What counts

Examples of things worth reporting:

- a message from a number that is not in `allowed_numbers` reaching the shell;
- the bot answering its own replies, or replaying old or edited messages;
- a way to make chat-term run a command the user didn't send;
- secrets or message text written to logs;
- the login store or config being created with loose file permissions.

Account bans by WhatsApp are a known risk of unofficial clients, not a
vulnerability. See the warning in the [README](README.md).

## Supported versions

Only the latest release gets fixes.
