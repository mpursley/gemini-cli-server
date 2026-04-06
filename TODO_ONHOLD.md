# TODO - gemini-cli-server

## Status Key

- [ ] TODO
- [I] In progress
- [C] Complete
- [x] Deployed to Prod/Active version (v1.1.6)

## Tasks

- [x] Re: /status Add the session "name" to the /status output?

- [x] when using /sessions I'd like to be able to add some text after to "grep" the session names for.. ( e.g. "/sessions fitness" -> only return sessions that include "fitness" in their name) (v1.1.4)

- [x] Can we add an slash options to:
  - [x] /repeat_last_reply ... To just repeat the last "reply" message... Sometimes ill loose connection during the thinking message and not get the reply message...
  - [x] /stop ... Sometimes i hit enter too soon and start a command. I can use /stop to stop the last message so i can fix it and re submit it?
  - [x] /run ... this command works perfectly, but does't show up in the telegram chat menu like the others do?

- [ ] cronjob: to allow gemini-cli-server to run commands on a schedule.

- [ ] memory:
  - [ ] to allow gemini-cli-server to refence chat logs from other sessions
  - [ ] if the memory folder is not yet setup, offer to create it, and interview the user to get a sense of what they want to remember.
  - [ ] implement a compaction, for sessions that are getting close to the context window limit

- [ ] cli: implement command-line interface for gemini-cli-server to allow for:
  - [ ] using the / commands from the command line
