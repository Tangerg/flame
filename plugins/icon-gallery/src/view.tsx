import { renderToStaticMarkup } from "react-dom/server.node";
import {
  Metadata,
  Panel,
  PanelTitle,
  TextField,
  Tile,
  TileGrid,
  Toolbar,
} from "@flame/ui";

type Drawing = {
  name: string;
  component: string;
  label: string;
  group: string;
  tags: readonly string[];
  svg: string;
};
// Static content needs no React client. Native search and the shared theme bootstrap remain live.
export function render(drawings: readonly Drawing[]): string {
  return (
    renderToStaticMarkup(
      <Panel label="Flame icon gallery">
        <Toolbar label="Icon filters">
          <PanelTitle>Flame icon gallery</PanelTitle>
          <TextField
            id="filter"
            type="search"
            aria-label="Filter icons"
            placeholder="Name or category"
          />
        </Toolbar>
        <Metadata>
          Original Flame drawings, shown with their canonical 24 px optical
          masters.
        </Metadata>
        <p id="status" role="status">
          Connecting…
        </p>
        <TileGrid id="icons" aria-label="Canonical icon drawings">
          {drawings.map((icon) => (
            <Tile
              key={icon.name}
              data-search={[
                icon.name,
                icon.component,
                icon.label,
                icon.group,
                ...icon.tags,
              ]
                .join(" ")
                .toLocaleLowerCase()}
              title={icon.label}
              visual={<span dangerouslySetInnerHTML={{ __html: icon.svg }} />}
            >
              <div>{icon.component}</div>
              <code>{icon.name}</code>
              <Metadata>{icon.group}</Metadata>
            </Tile>
          ))}
        </TileGrid>
      </Panel>,
    ) +
    `<script>
    const filter=document.getElementById('filter'),status=document.getElementById('status'),cards=[...document.querySelectorAll('article')];
    let connected=false;
    function render(){const query=filter.value.trim().toLocaleLowerCase();let count=0;for(const card of cards){card.hidden=!card.dataset.search.includes(query);if(!card.hidden)count++;}status.textContent=count+' of '+cards.length+' icons. Immutable package content.';}
    addEventListener('message',event=>{if(connected||event.source!==parent||event.data?.type!=='flame.view.connect.v3'||event.ports.length!==1)return;connected=true;const port=event.ports[0];port.start();if(event.data.initial?.format!=='flame.static.v1'){port.postMessage({type:'failure',reason:'Unsupported static view format.'});return;}render();});
    parent.postMessage('flame.view.ready.v3','*');filter.addEventListener('input',render);
  </script>`
  );
}
