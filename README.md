# nats-agent-core
Core NATS agent library for OLG

`nats-agent-core` is a shared Go library for agents that communicate over a NATS bus.

It provides common bus-facing functionality such as:
- NATS connection and reconnect handling
- JetStream and Key-Value access
- standard subject naming
- standard message envelopes
- configure and action submission helpers
- result and status publication helpers
- desired configuration storage and retrieval
- correlation using `rpc_id`

The library is **not a daemon**.  
It is meant to be used **inside long-running agents** such as:
- ucentral-client agent
- host agent
- VyOS agent

---

## Purpose

The goal of this library is to keep all common NATS/JetStream messaging logic in one reusable place, while leaving platform-specific logic inside the agents.

In simple words:

- **library** = common messaging and state helper
- **agent** = local business logic and execution

---

## What this library does

This library helps agents:

- connect to NATS
- reconnect after temporary disconnects
- create and use JetStream
- store desired configuration in JetStream Key-Value
- publish configure notifications
- publish action commands
- publish result and status messages
- subscribe to message subjects
- restore subscriptions after reconnect
- preserve request identity using `rpc_id`

---

## What this library does not do

This library does **not** implement workload-specific or platform-specific logic.

Examples of things that should stay outside this library:

- VyOS configuration translation
- host reboot/script execution
- trace or remote terminal implementation
- cloud-side business validation
- local apply / rollback logic

---

## Basic communication model

### Configure flow
1. Agent receives a validated configure request
2. Library stores desired configuration in JetStream KV
3. Library publishes a lightweight configure notification
4. Target agent receives the notification
5. Target agent loads the latest desired config from KV
6. Target agent applies it locally
7. Target agent publishes a result or status message

### Action flow
1. Agent receives a validated action request
2. Library publishes the action command on the target action subject
3. Target agent receives the action
4. Target agent executes the local action
5. Target agent publishes a result or status message

### Result flow
1. Target agent publishes result/status
2. Calling side receives the message through the library
3. Correlation is done using `rpc_id`

---

