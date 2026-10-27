// A pi extension that pi-setup.sh installs so pi.tape can show the review loop
// at the pace it usually runs:
//
//   - The session in the file NIT_DEMO_PRELUDE names, in which pi read the code
//     before recording, leads the history pi sends the model without showing
//     in its transcript, as if the screen had been cleared.
//   - Vertex AI sometimes takes several times as long as usual to start
//     answering, so a request that hasn't started in HEDGE_MS goes out again,
//     and pi gets whichever answers first.
import { readFileSync } from "node:fs";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

const HEDGE_MS = 2500;
const TRIES = 3;

export default function (pi: ExtensionAPI) {
	const file = process.env.NIT_DEMO_PRELUDE;
	if (file) {
		const prelude = readFileSync(file, "utf8")
			.split("\n")
			.filter(Boolean)
			.map((line) => JSON.parse(line))
			.filter((e) => e.type === "message" && e.message.role !== "system" && e.message.stopReason !== "error")
			.map((e) => e.message);
		pi.on("context", (event) => ({ messages: [...prelude, ...event.messages] }));
	}

	pi.on("session_start", () => {
		const fetch = globalThis.fetch;
		globalThis.fetch = (input, init) => {
			const url = input instanceof Request ? input.url : String(input);
			if (!url.includes("aiplatform.googleapis.com") || typeof init?.body !== "string") return fetch(input, init);
			return hedged(fetch, input, init);
		};
	});
}

function hedged(fetch: typeof globalThis.fetch, input: string | URL, init: RequestInit): Promise<Response> {
	return new Promise((resolve, reject) => {
		const tries: AbortController[] = [];
		let done = false;
		let failed = 0;
		const finish = () => {
			done = true;
			clearInterval(timer);
		};
		const send = () => {
			const ctl = new AbortController();
			tries.push(ctl);
			const signal = init.signal ? AbortSignal.any([init.signal, ctl.signal]) : ctl.signal;
			fetch(input, { ...init, signal }).then(
				(res) => {
					if (done) return ctl.abort();
					finish();
					for (const other of tries) if (other !== ctl) other.abort();
					resolve(res);
				},
				(err) => {
					if (++failed === tries.length && !done) {
						finish();
						reject(err);
					}
				},
			);
		};
		const timer = setInterval(() => tries.length < TRIES && send(), HEDGE_MS);
		send();
	});
}
