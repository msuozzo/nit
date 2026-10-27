// A pi extension, loaded by pi-setup.sh, that delivers nit's Send to the pi
// session it runs in. It serves a unix socket and sets NIT_SEND, which the nit
// skill's pane script passes on to nit, to post each comment there.
import { rmSync } from "node:fs";
import { createServer, type Server } from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

export default function (pi: ExtensionAPI) {
	const sock = join(tmpdir(), `pi-nit-${process.pid}.sock`);
	let server: Server | undefined;

	pi.on("session_start", (_event, ctx) => {
		if (server) return;
		rmSync(sock, { force: true });
		server = createServer((req, res) => {
			let body = "";
			req.setEncoding("utf8");
			req.on("data", (chunk) => (body += chunk));
			req.on("end", () => {
				try {
					const { prompt } = JSON.parse(body);
					// A comment sent while pi works waits for it to finish.
					pi.sendUserMessage(prompt, ctx.isIdle() ? undefined : { deliverAs: "followUp" });
					res.end("ok\n");
				} catch (e) {
					res.statusCode = 400;
					res.end(`${e}\n`);
				}
			});
		});
		server.listen(sock);
		process.env.NIT_SEND = `curl -sfS --unix-socket '${sock}' --data-binary @- http://pi/`;
	});

	pi.on("session_shutdown", () => {
		server?.close();
		server = undefined;
		rmSync(sock, { force: true });
	});
}
