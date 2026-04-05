import { createMcpHandler } from "agents/mcp";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";

interface Env {
  MEMORY_DB: D1Database;
  CHATS: KVNamespace;
}

async function hashToken(token: string): Promise<string> {
  const data = new TextEncoder().encode(token);
  const hash = await crypto.subtle.digest("SHA-256", data);
  return [...new Uint8Array(hash)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

interface MemoryRow {
  id: string;
  namespace: string;
  content: string;
  category: string;
  pinned: number;
  created_at: string;
  updated_at: string;
}

function createServer(db: D1Database) {
  const server = new McpServer({
    name: "memory",
    version: "1.0.0",
  });

  // ── recall ────────────────────────────────────────────────────────────
  // Retrieve memories for a namespace. Called at session start or when
  // prior context would help. Returns pinned items first, then matches.
  server.tool(
    "recall",
    "Search memories for a namespace. Use at session start to load project context, or when you need prior decisions/patterns. Returns pinned items first, then FTS matches or recent items.",
    {
      namespace: z.string().describe("Project or repo identifier, e.g. 'claudebox' or 'myapp'"),
      query: z.string().optional().describe("Search terms. Omit to get pinned + recent items."),
      category: z
        .enum(["decision", "pattern", "fact", "task", "preference"])
        .optional()
        .describe("Filter by category"),
      limit: z.number().min(1).max(50).default(20).describe("Max results"),
    },
    async ({ namespace, query, category, limit }) => {
      const params: unknown[] = [namespace];
      let sql: string;

      if (query) {
        // FTS search with BM25 ranking, pinned items boosted to top
        sql = `
          SELECT m.id, m.namespace, m.content, m.category, m.pinned,
                 m.created_at, m.updated_at
          FROM memories m
          JOIN memories_fts fts ON fts.rowid = m.rowid
          WHERE fts.content MATCH ?2
            AND m.namespace = ?1
        `;
        params.push(query);

        if (category) {
          sql += ` AND m.category = ?3`;
          params.push(category);
        }

        sql += ` ORDER BY m.pinned DESC, bm25(memories_fts) LIMIT ?${params.length + 1}`;
        params.push(limit);
      } else {
        // No query: return pinned first, then most recent
        sql = `
          SELECT id, namespace, content, category, pinned,
                 created_at, updated_at
          FROM memories
          WHERE namespace = ?1
        `;

        if (category) {
          sql += ` AND category = ?2`;
          params.push(category);
        }

        sql += ` ORDER BY pinned DESC, updated_at DESC LIMIT ?${params.length + 1}`;
        params.push(limit);
      }

      const { results } = await db.prepare(sql).bind(...params).all<MemoryRow>();

      if (!results.length) {
        return {
          content: [
            {
              type: "text" as const,
              text: `No memories found for namespace "${namespace}"${query ? ` matching "${query}"` : ""}.`,
            },
          ],
        };
      }

      const formatted = results
        .map((r) => {
          const pin = r.pinned ? " 📌" : "";
          return `[${r.id}] (${r.category}${pin}) ${r.content}`;
        })
        .join("\n\n");

      return {
        content: [
          {
            type: "text" as const,
            text: formatted,
          },
        ],
      };
    },
  );

  // ── remember ──────────────────────────────────────────────────────────
  // Store a memory. Use for architectural decisions, discovered patterns,
  // debugging insights, or task tracking across sessions.
  server.tool(
    "remember",
    "Store a memory. Use for decisions (with rationale), patterns, debugging insights, or cross-session task state. Store 'why' not 'what' — the code is the source of truth for 'what'.",
    {
      namespace: z.string().describe("Project or repo identifier"),
      content: z.string().describe("The memory content. Be concise but include rationale."),
      category: z
        .enum(["decision", "pattern", "fact", "task", "preference"])
        .describe(
          "decision: architectural/design choices with rationale. pattern: recurring code/workflow patterns. fact: project-specific facts (endpoints, credentials locations, etc). task: in-progress work to resume later. preference: user/team preferences.",
        ),
      pinned: z
        .boolean()
        .default(false)
        .describe("Pin to always appear at top of recall results. Use sparingly."),
    },
    async ({ namespace, content, category, pinned }) => {
      const result = await db
        .prepare(
          `INSERT INTO memories (namespace, content, category, pinned)
           VALUES (?1, ?2, ?3, ?4)
           RETURNING id`,
        )
        .bind(namespace, content, category, pinned ? 1 : 0)
        .first<{ id: string }>();

      return {
        content: [
          {
            type: "text" as const,
            text: `Stored memory ${result!.id} in ${namespace} (${category})`,
          },
        ],
      };
    },
  );

  // ── forget ────────────────────────────────────────────────────────────
  // Delete a memory by ID.
  server.tool(
    "forget",
    "Delete a memory by ID. Use to clean up stale decisions or completed tasks.",
    {
      id: z.string().describe("Memory ID from recall results"),
    },
    async ({ id }) => {
      const info = await db.prepare(`DELETE FROM memories WHERE id = ?1`).bind(id).run();

      if (!info.meta.changes) {
        return {
          content: [{ type: "text" as const, text: `No memory found with id "${id}"` }],
          isError: true,
        };
      }

      return {
        content: [{ type: "text" as const, text: `Deleted memory ${id}` }],
      };
    },
  );

  return server;
}

export default {
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    // Allow CORS preflight through
    if (request.method === "OPTIONS") {
      const server = createServer(env.MEMORY_DB);
      return createMcpHandler(server)(request, env, ctx);
    }

    // ── Service token auth (same KV lookup as gateway proxy) ─────────
    const serviceToken = request.headers.get("x-service-auth");
    if (!serviceToken) {
      return Response.json(
        { error: { message: "Missing x-service-auth header" } },
        { status: 401 },
      );
    }

    const hash = await hashToken(serviceToken);
    const tokenRecord = await env.CHATS.get(`service-token:${hash}`, "json") as { userId: string } | null;
    if (!tokenRecord) {
      return Response.json(
        { error: { message: "Invalid service token" } },
        { status: 401 },
      );
    }

    const server = createServer(env.MEMORY_DB);
    return createMcpHandler(server)(request, env, ctx);
  },
} satisfies ExportedHandler<Env>;
