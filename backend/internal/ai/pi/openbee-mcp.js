export default function (pi) {
  const url = process.env.OPENBEE_MCP_URL;
  if (!url) return;
  pi.registerMcpServer("openbee", {
    url,
    headers: { Authorization: `Bearer ${process.env.OPENBEE_API_KEY ?? ""}` },
    exposure: "direct",
  });
}
