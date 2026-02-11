# TODO

- [x] Implement the frontend in the svelte project in the @web/frontend folder (similar to the simple go html, but we're abandoning that and making it into a nice svelte project):
  - [x] in a "dashboard" folder in the routes, add the list of projects, conversations, and ratings
  - [x] under the dashboard folder, add a `conversations/[id]` page.
  - [x] Make the formatting very simple, without much css at all. I will style it later.
  - [x] Configure the svelte website to be fully static (client side only)
  - [x] Configure env vars for relevant URLs, like the go API server
  - [x] Add a dashboard.css file in the dashboard folder an import it into the +layout.svelte file
- [x] as part of the @plugins/claudecode/ plugin, have the model analyze the conversation based on the user's rating and note... keep in mind that the user may be rating either the prompt or the model's output (or both), but they will likely clarify in their note, so the model should take both into account. The model's analysis should be very short... no more than 2 sentences or a short bulleted list. It may require recommendations about how the original prompt could have been more clear or how the model should have known better or the model should have asked better questions. Its analysis should be technical and dry, no personality, and never snarky or arrogant.
- [x] Great, now take that feedback and add it to the POST to our go api server. In the @web/server/ accept that extra field as part of the rating, save it to the database (add a column in the rating table), and return it along with the rating object from the API. In the @web/frontend/ display the model's analysis along with the rating and user note.
- [ ] In the @web/server go server, monitor the claude code history.jsonl continuously and capture all projects, conversations, and turns (even if they don't have ratings yet). We need to track it all.
- [ ]
