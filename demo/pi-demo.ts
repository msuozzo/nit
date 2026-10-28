// A pi extension that pi-setup.sh installs so pi.tape can show the review loop
// at the pace it usually runs:
//
//   - The session in the file NIT_DEMO_PRELUDE names, in which pi read the code
//     before recording, leads the history pi sends the model without showing
//     in its transcript, as if the screen had been cleared.
//   - Vertex AI sometimes takes several times as long as usual to start
//     answering, so a request that hasn't started in HEDGE_MS goes out again,
//     and pi gets the first answer that isn't an error.
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

// pi gets the first try that answers without a 429 or 5xx, or the last try's
// error once all TRIES have failed. A try that fails while none other is
// pending sends the next one right away.
function hedged(fetch: typeof globalThis.fetch, input: string | URL, init: RequestInit): Promise<Response> {
	return new Promise((resolve, reject) => {
		const tries: AbortController[] = [];
		let done = false;
		let failed = 0;
		const finish = (ctl: AbortController, give: () => void) => {
			done = true;
			clearInterval(timer);
			for (const other of tries) if (other !== ctl) other.abort();
			give();
		};
		const fail = (ctl: AbortController, give: () => void) => {
			if (done) return;
			if (++failed === TRIES || init.signal?.aborted) return finish(ctl, give);
			ctl.abort();
			if (failed === tries.length) send();
		};
		const send = () => {
			const ctl = new AbortController();
			tries.push(ctl);
			const signal = init.signal ? AbortSignal.any([init.signal, ctl.signal]) : ctl.signal;
			fetch(input, { ...init, signal }).then(
				(res) => {
					if (done) return ctl.abort();
					if (res.status === 429 || res.status >= 500) return fail(ctl, () => resolve(res));
					finish(ctl, () => resolve(res));
				},
				(err) => fail(ctl, () => reject(err)),
			);
		};
		const timer = setInterval(() => tries.length < TRIES && send(), HEDGE_MS);
		send();
	});
}
