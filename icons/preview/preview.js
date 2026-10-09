import { icons } from "../catalog.js";
import { opticalSize } from "../react/opticalSize.js";

const drawings = new Map(
  await Promise.all(
    icons.flatMap((icon) =>
      [16, 24].map(async (grid) => [
        `${grid}/${icon.name}`,
        await fetch(`../${icon.svg[grid]}`).then((response) => {
          if (!response.ok) throw new Error(`failed to load ${icon.name}`);
          return response.text();
        }),
      ]),
    ),
  ),
);
const groups = [...new Set(icons.map((icon) => icon.group))].sort();
let master = 16;
let selected = icons.find((icon) => icon.name === "brain-off");
const sizes = [9, 12, 14, 16, 20, 21, 22, 24, 28, 36];

function markup(icon, grid, size) {
  return drawings
    .get(`${grid}/${icon.name}`)
    .replace(
      "<svg ",
      `<svg aria-hidden="true" style="width:${size}px;height:${size}px" `,
    );
}
function renderLive() {
  const size = Number(document.querySelector("#size").value);
  const grid = opticalSize(size);
  document.querySelector("#live").innerHTML =
    markup(selected, grid, size) +
    `<span id="size-label">${size}px · ${grid}px master</span>`;
}
function renderInspector() {
  document.querySelector("#name").textContent = selected.component;
  document.querySelector("#metadata").textContent =
    `${selected.group} / ${selected.tags.join(" · ") || selected.label}`;
  document.querySelector("#masters").innerHTML = [16, 24]
    .map(
      (grid) =>
        `<div class="master">${markup(selected, grid, 80)}<a href="../${selected.svg[grid]}" download>${grid}px SVG ↓</a></div>`,
    )
    .join("");
  document.querySelector("#actual").innerHTML = sizes
    .map(
      (size) =>
        `<div class="actual-item"><div class="actual-mark">${markup(selected, opticalSize(size), size)}</div><span>${size}px</span></div>`,
    )
    .join("");
  document.querySelector("#api").textContent =
    `import { ${selected.component} }\n  from "@flame/icons/react";\n\n<${selected.component} size={16} />`;
  document.querySelector("#context").innerHTML =
    `<div class="section-label">IN CONTEXT</div><div class="context-row">${markup(selected, 16, 16)}<span>${selected.label}</span></div><div class="context-pill">${markup(selected, 16, 14)} ${selected.label}</div>`;
  renderLive();
}
function renderGallery() {
  const query = document.querySelector("#search").value.trim().toLowerCase();
  const group = document.querySelector("#group").value;
  const filtered = icons.filter(
    (icon) =>
      (!group || icon.group === group) &&
      [icon.name, icon.component, icon.label, ...icon.tags]
        .join(" ")
        .toLowerCase()
        .includes(query),
  );
  document.querySelector("#gallery").innerHTML = filtered.length
    ? groups
        .map((group) => {
          const members = filtered.filter((icon) => icon.group === group);
          return members.length
            ? `<div class="group"><h2>${group}<span>${members.length}</span></h2><div class="grid">${members
                .map(
                  (icon) =>
                    `<button class="glyph" data-icon="${icon.name}" aria-label="Inspect ${icon.component}" aria-pressed="${icon === selected}" title="${icon.component}">${markup(icon, master, 32)}<span>${icon.component}</span></button>`,
                )
                .join("")}</div></div>`
            : "";
        })
        .join("")
    : '<p class="empty">No icons match your search.</p>';
  document.querySelector("#count").textContent =
    `${filtered.length} of ${icons.length} icons · ${icons.length * 2} optical drawings`;
  document
    .querySelectorAll("[data-master]")
    .forEach((button) =>
      button.setAttribute(
        "aria-pressed",
        String(Number(button.dataset.master) === master),
      ),
    );
}
for (const group of groups) {
  const option = document.createElement("option");
  option.value = group;
  option.textContent = group;
  document.querySelector("#group").append(option);
}
document.querySelector("#inventory").textContent =
  `${icons.length} authored icons · Independent 16px and 24px drawings`;
document.querySelector("#gallery").addEventListener("click", (event) => {
  const button = event.target.closest("[data-icon]");
  if (!button) return;
  selected = icons.find((icon) => icon.name === button.dataset.icon);
  document
    .querySelectorAll("[data-icon]")
    .forEach((item) =>
      item.setAttribute("aria-pressed", String(item === button)),
    );
  renderInspector();
  if (window.matchMedia("(max-width: 720px)").matches) {
    document.querySelector("#inspector").scrollIntoView({ block: "start" });
  }
});
document.querySelectorAll("[data-master]").forEach((button) =>
  button.addEventListener("click", () => {
    master = Number(button.dataset.master);
    renderGallery();
  }),
);
document.querySelector("#search").addEventListener("input", renderGallery);
document.querySelector("#group").addEventListener("change", renderGallery);
document.querySelector("#size").addEventListener("input", renderLive);
document.querySelector("#theme").addEventListener("click", (event) => {
  const dark = document.documentElement.dataset.theme !== "dark";
  document.documentElement.dataset.theme = dark ? "dark" : "light";
  event.currentTarget.textContent = dark ? "Light preview" : "Dark preview";
  event.currentTarget.setAttribute("aria-pressed", String(dark));
});
renderGallery();
renderInspector();
