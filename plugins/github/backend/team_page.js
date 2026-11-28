const token = location.hash.slice(1);
if (token) history.replaceState(null, "", location.pathname);
const title = document.getElementById("title");
const message = document.getElementById("status");
const source = document.getElementById("source");
const evidence = document.getElementById("evidence");
const head = document.getElementById("head");
const checks = document.getElementById("checks");
const report = document.getElementById("report");
const approve = document.getElementById("approve");
const receipt = document.getElementById("receipt");

function setLink(anchor, value) {
  anchor.hidden = true;
  anchor.removeAttribute("href");
  if (!value) return;
  try {
    const url = new URL(value);
    if (url.protocol !== "https:" || url.hostname !== "github.com") return;
    anchor.href = url.href;
    anchor.hidden = false;
  } catch {
    // A malformed link carries no navigation authority.
  }
}

function clearEvidence() {
  approval = undefined;
  evidence.hidden = true;
  approve.hidden = true;
  head.textContent = "";
  checks.textContent = "";
  report.textContent = "";
  setLink(receipt);
}

async function call(resource, body) {
  return fetch(`/flame/team/${resource}`, {
    method: body ? "POST" : "GET",
    cache: "no-store",
    redirect: "error",
    headers: {
      Authorization: `Bearer ${token}`,
      ...(body && { "Content-Type": "application/json" }),
    },
    body: body && JSON.stringify(body),
  });
}

let approval;
let busy = false;
async function refresh() {
  if (busy) return;
  busy = true;
  try {
    const result = await call("task");
    if (!result.ok) throw new Error("Task access is unavailable.");
    const task = await result.json();
    title.textContent = task.title;
    setLink(source, task.url);
    message.textContent = task.status;
    clearEvidence();
    if (!task.hasEvidence) return;
    const details = await call("evidence");
    if (!details.ok) {
      message.textContent = "Evidence access is unavailable.";
      return;
    }
    const value = await details.json();
    evidence.hidden = false;
    head.textContent = value.head;
    checks.textContent = JSON.stringify(
      { project: value.project, ci: value.ci },
      null,
      2,
    );
    report.textContent = value.draft ?? "";
    if (value.publication === "unconfirmed")
      message.textContent = "Original publication is unconfirmed.";
    if (value.publication === "refused")
      message.textContent = "The original publication was refused.";
    if (value.receipt) {
      message.textContent = "The original publication was acknowledged.";
      setLink(receipt, value.receipt.comment.url);
    }
    if (value.canPublish && value.digest) {
      approval = { id: task.id, digest: value.digest };
      approve.textContent =
        value.publication === "unconfirmed"
          ? "Check original publication"
          : "Approve and publish to GitHub";
      approve.hidden = false;
    }
  } catch {
    title.textContent = "Flame Issue approval";
    message.textContent = "Task access is unavailable.";
    setLink(source);
    clearEvidence();
  } finally {
    busy = false;
  }
}

approve.addEventListener("click", async () => {
  if (!approval || busy) return;
  busy = true;
  approve.disabled = true;
  try {
    const result = await call("approve", approval);
    if (result.status === 202) {
      message.textContent = "Original publication is unconfirmed.";
    } else if (!result.ok) {
      clearEvidence();
      message.textContent =
        result.status === 409
          ? "The verification changed. Review the current report."
          : "Publication access is unavailable.";
    } else {
      const value = await result.json();
      approve.hidden = true;
      message.textContent = "The original publication was acknowledged.";
      setLink(receipt, value.comment.url);
    }
  } catch {
    message.textContent = "Could not confirm the original publication.";
  } finally {
    busy = false;
    approve.disabled = false;
  }
});

if (token) {
  void refresh();
  window.addEventListener("focus", refresh);
  window.setInterval(() => {
    if (document.visibilityState === "visible") void refresh();
  }, 3000);
} else {
  message.textContent = "Open a current invitation to view this Issue.";
}
