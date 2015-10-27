---
     _______  _______  ______   _______  _______  _______
    |  _    ||       ||      | |       ||       ||   _   |
    | |_|   ||   _   ||  _    ||    ___||    ___||  |_|  |
    |       ||  | |  || | |   ||   |___ |   | __ |       |
    |  _   | |  |_|  || |_|   ||    ___||   ||  ||       |
    | |_|   ||       ||       ||   |___ |   |_| ||   _   |
    |_______||_______||______| |_______||_______||__| |__|
---

_For when you don't need a whole [Supermarket](https://supermarket.chef.io)..._

# What?

Bodega is intended to be a light proxy in front of Chef Server, offering a
Berkshelf `/universe` endpoint and proxied cookbook downloads.

# Why?

If you just want to be able to resolve your internal cookbooks without
providing Chef Server credentials to everyone, but standing up a full
Supermarket instance is more than you need, Bodega may be for you.

# How

`go get github.com/ewr/chef-bodega`

`chef-bodega --help`

Command flags:

* __baseURL:__ Base to use in our download URLs. Could be "http://localhost:8080"
    if you're just playing around locally, or some different host/port for
    network use.
* __chef.server:__ Chef server URL, including organization path if necessary.
* __chef.client:__ Chef client name
* __chef.pem:__ Path to PEM file for Chef client
* __chef.interval:__ Interval at which to poll for new cookbooks
* __listen:__ Listening address (":8080" by default)
* __skip-ssl:__ Skip SSL validation for Chef Server? (defaults to true)


# TODO

__Everything.__ This is a barely-working proof-of-concept, but I think it
serves a legit need. It will serve up your cookbooks securely (in terms of
not giving others access to Chef Server), but you shouldn't expose it to
untrusted users.

* Tests!
* Concurrent fetches for cookbook version info?
* Cache cookbook tarballs?

# An Alternate Approach

This tool was renamed Bodega after a naming collision with
[Minimart](https://github.com/electric-it/minimart), another tool that
attempts to solve the Supermarket-lite use case. Minimart creates a static
representation of the cookbook tree, so it may better support uses with lots
of download traffic.

# Who

Bodega was written by [Eric Richardson](http://ewr.is), to scratch an
itch felt at [Southern California Public Radio](http://www.scpr.org).
