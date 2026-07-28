package dreaming

// Every prompt here asks for a bare JSON object. The replies are parsed by
// machine and never shown to the user, so prose around the JSON is pure
// breakage — hence the repeated, blunt instruction at the end of each.

// reflexSystem drives the after-every-reply capture.
//
// The hard part is not extraction, it is restraint. A model asked "what is worth
// remembering here?" will find something in literally every exchange, and an
// agent whose memory fills with "the user asked about the weather" is worse off
// than one with no memory at all: the boot summary is a fixed number of lines,
// so junk crowds out what matters.
const reflexSystem = `You maintain the long-term memory of an AI agent.

You will be shown one exchange: what the user said, and how the agent replied.
Your job is to decide whether anything in it is worth remembering permanently,
and to write it down if so.

REMEMBER things that stay true after this conversation ends:
- Facts about the user: their name, role, timezone, employer, projects, the
  people and systems they work with.
- Stable preferences: how they want things done, tools they use, things they
  have said they dislike.
- Decisions and commitments: what was chosen, what was ruled out, and why.
- Durable project state: repository names, deploy targets, credentials'
  locations (never the values), architecture decisions, deadlines.
- Corrections the user made to the agent. These matter most — the same mistake
  repeated after being corrected is the thing memory exists to prevent.

DO NOT remember:
- The fact that a question was asked or a task was performed. The chat log
  already holds that.
- Anything the agent generated that is reproducible on demand: code it wrote,
  summaries, explanations, search results.
- Transient state: what a command printed, whether a build passed, what time it
  was, what is currently open.
- Anything already covered by the existing memories you are shown.
- Secret values: passwords, API keys, tokens. Record that a secret exists and
  where it lives, never what it is.

Most exchanges contain nothing worth keeping. Returning an empty list is the
correct, common answer — prefer it over recording something marginal.
Save at most 3 memories, and only ones you would stand behind a month from now.

Write each memory as a standalone statement that makes sense with no
conversation around it. "Wynter prefers Go for backend services" — not "he
prefers that".

Importance, used to decide what an agent sees at the start of every future
conversation:
  9-10  identity and standing instructions that must never be missed
  7-8   strong preferences, key project facts, corrections
  5-6   useful context worth having
  1-4   minor detail

Reply with ONLY a JSON object, no prose and no markdown fences:
{"memories":[{"content":"...","summary":"one line","category":"fact|preference|project|decision|person|general","importance":7,"tags":"comma,separated"}]}
Return {"memories":[]} when nothing qualifies.`

// extractSystem drives the per-thread pass of a dream. Same judgement as the
// reflex, but over a whole conversation at once and looking for the arc of it
// rather than a single turn.
const extractSystem = `You are reading one past conversation on behalf of the AI
agent that took part in it, looking for what is worth remembering permanently.

Extract durable facts only — things that will still be true and still be useful
long after this conversation is forgotten:
- Who the user is and how they work.
- Stated preferences, standing instructions, and corrections they made.
- Decisions reached, options rejected, and the reasoning behind both.
- Project and system facts: names, locations, structures, constraints, dates.
- Unresolved problems and open commitments that outlive the conversation.

Ignore entirely:
- The narrative of the conversation itself ("the user asked X, the agent did Y").
- Generated output: code, summaries, explanations, command results.
- Transient state and anything time-bound that has already passed.
- Secret values. Note that a secret exists and where, never what it is.

A conversation that was purely a task being carried out yields nothing. Say so
by returning an empty list rather than manufacturing facts to fill it.
Return at most 8 facts, each a standalone statement.

Reply with ONLY a JSON object, no prose and no markdown fences:
{"facts":[{"content":"...","summary":"one line","category":"fact|preference|project|decision|person|general","importance":7,"tags":"comma,separated"}]}`

// consolidateSystem drives the second half of a dream: reconciling the facts
// just harvested against the memories already stored.
//
// This is the only place in the app that deletes memories on its own, so the
// prompt spends most of its length on when NOT to. The caller enforces its own
// limits on top of this (see applyOps) — a prompt is guidance, not a guarantee.
const consolidateSystem = `You are consolidating the long-term memory of an AI
agent, the way sleep consolidates a day's experience: new observations are
merged into what is already known, duplicates collapse, and what no longer holds
is let go.

You are given NEW FACTS harvested from conversations the agent has not reviewed
before, and EXISTING MEMORIES already in its database (each with an id).

Decide, for the whole set:

ADD — a new fact that is genuinely new. If an existing memory already covers it,
update that one instead of adding a near-duplicate.

UPDATE — an existing memory that a new fact refines, corrects, extends, or that
several memories should be merged into. Merging is the main win here: three
memories saying the same thing three ways should become one clear memory, with
the others forgotten. Give the full replacement text, not a diff. Raise the
importance of anything that keeps coming up; lower it for anything that has
turned out not to matter.

FORGET — remove a memory only when one of these is true:
- It is a duplicate of another memory you are keeping or updating.
- It has been directly superseded: the new facts show it is now wrong.
- It was never durable — a transient detail that should not have been saved.
Do NOT forget something merely because it is old, or narrow, or has not come up
lately. Age is not staleness. When in doubt, keep it.

Never let a fact vanish in a merge: if you forget three memories in favour of
one, that one must carry everything the three said that still holds.

Also write a short summary — two or three sentences, plain language — of what
changed and why, for the user to read.

Reply with ONLY a JSON object, no prose and no markdown fences:
{
  "add":[{"content":"...","summary":"one line","category":"...","importance":7,"tags":"a,b"}],
  "update":[{"id":"existing-id","content":"...","summary":"one line","category":"...","importance":8,"tags":"a,b"}],
  "forget":[{"id":"existing-id","reason":"duplicate of ..."}],
  "summary":"what changed and why"
}
Any of the three lists may be empty.`
