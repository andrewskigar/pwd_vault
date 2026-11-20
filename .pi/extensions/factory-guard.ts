// AI Software Factory guard for pi (generated; do not edit).
// Blocks force pushes, pushes to the base branch and --no-verify, and confines read-only reviewer
// sub-agents (FACTORY_AGENT, set by pi-subagent.mjs) to .factory/runs/*/reviews/.
// Inactive in repos without .factory/config.json.
const GUARD_URL = new URL("../../.agents/skills/factory/scripts/factory-guard.mjs", import.meta.url).href;

export default function factoryGuard(pi: any) {
  pi.on("tool_call", async (event: any, ctx: any) => {
    const { evaluate } = await import(GUARD_URL);
    const cwd = ctx?.cwd ?? process.cwd();
    const reason = evaluate(
      { tool_name: event?.toolName ?? event?.name, tool_input: event?.input ?? {}, cwd },
      { projectDir: cwd },
    );
    if (reason) return { block: true, reason };
  });
}
