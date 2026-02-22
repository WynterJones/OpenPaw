import { useEffect } from 'react';
import { useOutletContext } from 'react-router';
import type { TocItem } from '../../components/docs/DocsTableOfContents';
import {
  Diagram,
  DiagramBox,
  DiagramArrow,
  DiagramRow,
  DiagramStack,
} from '../../components/docs/DocsDiagram';

interface DocsContext {
  registerToc: (items: TocItem[]) => void;
}

export function HowItWorks() {
  const { registerToc } = useOutletContext<DocsContext>();

  useEffect(() => {
    registerToc([
      { id: 'overview', text: 'Overview', level: 2 },
      { id: 'request-flow', text: 'Request Flow', level: 2 },
      { id: 'gateway-routing', text: 'Gateway Routing', level: 2 },
      { id: 'agent-lifecycle', text: 'Agent Lifecycle', level: 2 },
      { id: 'tool-execution', text: 'Tool Execution', level: 2 },
      { id: 'streaming', text: 'Real-time Streaming', level: 2 },
    ]);
  }, [registerToc]);

  return (
    <>
      <h1>How It Works</h1>
      <p className="text-lg text-text-2 mb-8">
        Understand how OpenPaw processes messages, routes them to agents, and streams responses
        back to the browser in real-time.
      </p>

      <h2 id="overview">Overview</h2>
      <p>
        OpenPaw is a <strong>single Go binary</strong> that embeds a React frontend, SQLite database,
        and a multi-agent AI system. When a user sends a message, it flows through several layers
        before reaching the AI and streaming back.
      </p>

      <Diagram title="System Architecture">
        <DiagramStack>
          <DiagramBox variant="primary">Browser (React + WebSocket)</DiagramBox>
          <DiagramArrow direction="down" />
          <DiagramBox variant="accent">Go HTTP Server (chi router)</DiagramBox>
          <DiagramArrow direction="down" />
          <DiagramRow>
            <DiagramBox variant="muted">Gateway Router</DiagramBox>
            <DiagramBox variant="muted">SQLite Database</DiagramBox>
            <DiagramBox variant="muted">Tool Manager</DiagramBox>
          </DiagramRow>
          <DiagramArrow direction="down" />
          <DiagramBox variant="primary">Claude API (Anthropic)</DiagramBox>
        </DiagramStack>
      </Diagram>

      <h2 id="request-flow">Request Flow</h2>
      <p>Every chat message follows this path:</p>
      <ol>
        <li><strong>User sends message</strong> via WebSocket connection</li>
        <li><strong>Server receives</strong> the message and creates a work order</li>
        <li><strong>Gateway analyzes</strong> the message to determine which agent should handle it</li>
        <li><strong>Agent processes</strong> the message with its system prompt, tools, and context</li>
        <li><strong>Response streams</strong> back via WebSocket as text deltas</li>
        <li><strong>Message stored</strong> in SQLite with cost/token metadata</li>
      </ol>

      <Diagram title="Message Flow">
        <DiagramStack>
          <DiagramBox variant="primary">User Message</DiagramBox>
          <DiagramArrow direction="down" label="websocket" />
          <DiagramBox variant="accent">Work Order Created</DiagramBox>
          <DiagramArrow direction="down" label="routing" />
          <DiagramBox variant="accent">Gateway Analysis</DiagramBox>
          <DiagramArrow direction="down" label="dispatch" />
          <DiagramBox variant="primary">Agent Execution</DiagramBox>
          <DiagramArrow direction="down" label="stream" />
          <DiagramBox variant="muted">Response + Storage</DiagramBox>
        </DiagramStack>
      </Diagram>

      <h2 id="gateway-routing">Gateway Routing</h2>
      <p>
        The <strong>Gateway</strong> is a special meta-agent that analyzes incoming messages and
        decides which agent should handle them. It uses a system prompt to understand the capabilities
        of each registered agent.
      </p>
      <p>Routing decisions are based on:</p>
      <ul>
        <li><strong>Explicit mentions</strong> — <code>@agent-name</code> routes directly to that agent</li>
        <li><strong>Content analysis</strong> — the gateway reads the message and matches it to agent descriptions</li>
        <li><strong>Thread context</strong> — agents already in a conversation get priority for follow-ups</li>
        <li><strong>Default fallback</strong> — messages without a clear match go to the general assistant</li>
      </ul>

      <h2 id="agent-lifecycle">Agent Lifecycle</h2>
      <p>Each agent in OpenPaw follows this lifecycle:</p>
      <ol>
        <li><strong>Configuration</strong> — defined with a name, system prompt, model, and tool access</li>
        <li><strong>Activation</strong> — when routed a message, the agent is activated with context</li>
        <li><strong>Processing</strong> — the agent calls the Claude API with its prompt + conversation history</li>
        <li><strong>Tool Use</strong> — if the agent needs tools, it can call them mid-response</li>
        <li><strong>Completion</strong> — the response is finalized and stored</li>
      </ol>
      <p>
        Agents are stateless between requests — all context comes from the conversation thread and
        the Context system (uploaded files and knowledge).
      </p>

      <h2 id="tool-execution">Tool Execution</h2>
      <p>
        Tools are separate processes managed by OpenPaw. Each tool exposes HTTP endpoints that agents
        can call during their execution.
      </p>
      <ul>
        <li>Tools are compiled Go binaries, started on-demand</li>
        <li>Each tool gets a unique port and health check endpoint</li>
        <li>Agents call tools via the internal tool proxy</li>
        <li>Tool output is included in the agent&apos;s response stream</li>
        <li>Integrity verification ensures tools haven&apos;t been tampered with</li>
      </ul>

      <Diagram title="Tool Execution">
        <DiagramStack>
          <DiagramBox variant="primary">Agent needs data</DiagramBox>
          <DiagramArrow direction="down" label="tool_call" />
          <DiagramBox variant="accent">Tool Manager</DiagramBox>
          <DiagramArrow direction="down" label="http" />
          <DiagramRow>
            <DiagramBox variant="muted">Weather API</DiagramBox>
            <DiagramBox variant="muted">GitHub API</DiagramBox>
            <DiagramBox variant="muted">Database</DiagramBox>
          </DiagramRow>
          <DiagramArrow direction="down" label="result" />
          <DiagramBox variant="primary">Agent continues response</DiagramBox>
        </DiagramStack>
      </Diagram>

      <h2 id="streaming">Real-time Streaming</h2>
      <p>
        OpenPaw uses <strong>WebSockets</strong> for real-time communication. When an agent starts
        responding, text is streamed token-by-token to the browser:
      </p>
      <ul>
        <li><code>text_delta</code> — partial text as it&apos;s generated</li>
        <li><code>tool_start</code> — an agent begins calling a tool</li>
        <li><code>tool_delta</code> — partial tool output</li>
        <li><code>tool_end</code> — tool call completed</li>
        <li><code>turn_complete</code> — the agent has finished its turn</li>
        <li><code>result</code> — final message with cost and token metadata</li>
      </ul>
      <p>
        This streaming architecture means users see responses appear in real-time, just like
        typing in a chat application.
      </p>
    </>
  );
}
