# Tools Integration Guide

This document describes the current integration of the tools portal and member tools.

## Access tiers

- `Public`: available without a session.
- `Member`: any valid signed-in account; anonymous page requests redirect to `/login.html?next=...`, API requests return JSON 401.
- `OwnerOnly`: visible only to the configured site owner. The development owner override is separate and does not grant member access.

## History

History is mounted at `/tool/history/` and requires login. It is scoped to the current user in the shared SQLite database. It includes imported browser/Bilibili archives and authenticated visits to article, forum-post, course, and file-detail pages on this site. It cannot read a visitor's complete local browser history; archives must be imported explicitly.

The tool keeps the upstream history SPA routes under `/tool/history/browser/`, `/bilibili/`, `/favorite/`, and `/backup/`. The root route is the Wasteland site-footprint overview, presented inside the site's navigation and visual shell.

## Private bookmarks

`/tool/favorite/` is a per-account URL bookmark manager backed by `tool_bookmarks`. CRUD operations are scoped by `user_id`. Only HTTP and HTTPS URLs are accepted.
