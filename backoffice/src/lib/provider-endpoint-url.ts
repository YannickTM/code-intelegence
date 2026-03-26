export function isValidProviderEndpointURL(value: string) {
  try {
    const url = new URL(value);
    const port = url.port === "" ? undefined : Number(url.port);
    return (
      (url.protocol === "http:" || url.protocol === "https:") &&
      url.username === "" &&
      url.password === "" &&
      url.search === "" &&
      url.hash === "" &&
      url.hostname !== "" &&
      (port === undefined ||
        (Number.isInteger(port) && port >= 1 && port <= 65535))
    );
  } catch {
    return false;
  }
}
