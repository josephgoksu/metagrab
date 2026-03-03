import { Container } from "@cloudflare/containers";

interface Env {
  METAGRAB_CONTAINER: DurableObjectNamespace<MetagrabContainer>;
}

export class MetagrabContainer extends Container {
  defaultPort = 8080;
  sleepAfter = "5m";

  // API_KEY is injected via `wrangler secret put API_KEY`
  override get envVars(): Record<string, string> {
    return {
      API_KEY: (this.env as Record<string, string>).API_KEY ?? "",
    };
  }
}

/**
 * Pick a random container instance from the pool for simple load balancing.
 */
async function getRandom(
  ns: DurableObjectNamespace<MetagrabContainer>,
  maxInstances: number,
): Promise<DurableObjectStub<MetagrabContainer>> {
  const idx = Math.floor(Math.random() * maxInstances);
  const id = ns.idFromName(`instance-${idx}`);
  return ns.get(id);
}

export default {
  async fetch(
    request: Request,
    env: Env,
  ): Promise<Response> {
    const container = await getRandom(env.METAGRAB_CONTAINER, 3);
    return container.fetch(request);
  },
};
