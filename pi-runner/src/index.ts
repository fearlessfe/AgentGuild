import { runAnalyzer } from "./analyzer.js";
import { emptyUsage, parseAnalysisJob, PROTOCOL_VERSION, type AnalysisResult } from "./protocol.js";

async function main(): Promise<void> {
  let raw: string;
  try {
    raw = "";
    process.stdin.setEncoding("utf8");
    for await (const chunk of process.stdin) raw += chunk;
    const job = parseAnalysisJob(JSON.parse(raw));
    const controller = new AbortController();
    const abort = () => controller.abort();
    process.once("SIGTERM", abort);
    process.once("SIGINT", abort);
    const options = {
      signal: controller.signal,
      ...(process.env.PI_MODEL_API_KEY ? { runtimeApiKey: process.env.PI_MODEL_API_KEY } : {}),
    };
    const result = await runAnalyzer(job, options);
    process.stdout.write(`${JSON.stringify(result)}\n`);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    const result: AnalysisResult = {
      protocol_version: PROTOCOL_VERSION,
      job_id: "unknown",
      agent_version_id: "unknown",
      status: "invalid_request",
      usage: emptyUsage(),
      audit_events: [],
      error_code: "INVALID_REQUEST",
      error_message: message.slice(0, 2000),
    };
    process.stdout.write(`${JSON.stringify(result)}\n`);
    process.exitCode = 2;
  }
}

await main();
