// The one module of the fixture that is allowed to reach the network: core/
// owns the timeout, the double submit header and the Problem Details reader of
// the product, so a page never fetches on its own.
export async function request(path) {
    const response = await fetch(path, { headers: { Accept: "application/json" } });
    if (!response.ok) {
        throw new Error(`request failed with ${response.status}`);
    }
    return response.json();
}
