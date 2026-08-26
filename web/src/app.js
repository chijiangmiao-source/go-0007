const ids = ["stations", "arcs", "targets", "solutions", "versions"];
const checkpoint = document.querySelector("#checkpoint");
const events = document.querySelector("#events");

async function loadState() {
  const [stateResponse, eventsResponse] = await Promise.all([
    fetch("/v1/system/state"),
    fetch("/v1/system/events?limit=8"),
  ]);
  if (!stateResponse.ok || !eventsResponse.ok) {
    throw new Error("backend state request failed");
  }
  const state = await stateResponse.json();
  const eventPage = await eventsResponse.json();
  for (const id of ids) {
    document.querySelector(`#${id}`).textContent = String(state[id] ?? 0);
  }
  checkpoint.textContent = `检查点 ${state.checkpoint?.last_applied_seq ?? 0} · ${state.checkpoint?.state_hash ?? "pending"}`;
  events.replaceChildren(
    ...eventPage.events.map((event) => {
      const item = document.createElement("li");
      item.innerHTML = `<span>${event.seq}</span><strong>${event.type}</strong><code>${event.aggregate_id}</code>`;
      return item;
    }),
  );
}

document.querySelector("#refresh").addEventListener("click", () => {
  loadState().catch((error) => {
    checkpoint.textContent = error.message;
  });
});

loadState().catch((error) => {
  checkpoint.textContent = error.message;
});
