# FotoJournal Product Requirements Document

## Document control

| Field         | Detail                                         |
| ------------- | ---------------------------------------------- |
| Product       | FotoJournal                                    |
| Document type | Product Requirements Document                  |
| Status        | Draft for MVP build                            |
| Owner         | Paul Deegan                                    |
| Product type  | Web and PWA SaaS application for photographers |
| Version       | 0.1                                            |
| Date          | 8 July 2026                                    |

## Table of contents

- [1. Product summary](#1-product-summary)
- [2. Product vision](#2-product-vision)
- [3. Goals](#3-goals)
  - [3.1 MVP goals](#31-mvp-goals)
  - [3.2 Business goals](#32-business-goals)
  - [3.3 Technical goals](#33-technical-goals)
- [4. Non-goals for MVP](#4-non-goals-for-mvp)
- [5. Target users](#5-target-users)
  - [5.1 Amateur photographer](#51-amateur-photographer)
  - [5.2 Professional photographer with personal work](#52-professional-photographer-with-personal-work)
  - [5.3 Serious enthusiast](#53-serious-enthusiast)
- [6. Primary user problems](#6-primary-user-problems)
- [7. Product principles](#7-product-principles)
- [8. MVP feature requirements](#8-mvp-feature-requirements)
- [9. User experience requirements](#9-user-experience-requirements)
- [10. Technical architecture](#10-technical-architecture)
- [11. Proposed repository structure](#11-proposed-repository-structure)
- [12. Data model draft](#12-data-model-draft)
- [13. API requirements](#13-api-requirements)
- [14. Security and privacy requirements](#14-security-and-privacy-requirements)
- [15. Performance requirements](#15-performance-requirements)
- [16. Offline requirements](#16-offline-requirements)
- [17. MVP user journeys](#17-mvp-user-journeys)
- [18. MVP success metrics](#18-mvp-success-metrics)
- [19. Release plan](#19-release-plan)
- [20. Suggested implementation PR sequence](#20-suggested-implementation-pr-sequence)
- [21. Risks and mitigations](#21-risks-and-mitigations)
- [22. Open questions](#22-open-questions)
- [23. Recommended MVP cut](#23-recommended-mvp-cut)
- [24. Definition of done for MVP](#24-definition-of-done-for-mvp)
- [25. Final recommendation](#25-final-recommendation)

## 1. Product summary

FotoJournal is a web and PWA application for photographers who want to plan, track, and reflect on their personal photography projects.

The product is designed for amateur and professional photographers who shoot for personal creative fulfilment, not client work. It gives them a private, organised space to plan projects, record ideas, capture locations, upload images, track project progress, and build a meaningful archive of their creative work.

The MVP will focus on personal project planning, private journaling, image uploads, location privacy, EXIF metadata, and PWA usability. AI features will be deferred until the core product is built and validated.

FotoJournal is intended to become a SaaS product.

## 2. Product vision

Photographers often have personal projects scattered across notebooks, phone notes, image folders, map pins, weather apps, and memory. FotoJournal brings those fragments into one calm, photography-focused workspace.

The product should help photographers answer questions like these.

- What personal projects am I working on?
- What was I trying to capture?
- Where did I shoot?
- What locations do I want to revisit?
- What settings, gear, light, and weather shaped the image?
- What still needs to be photographed?
- How has this project developed over time?

FotoJournal should feel less like a task manager and more like a creative companion. It should respect privacy, support field use, and feel purpose-built for photography rather than being a generic notes app with image uploads stapled to the side.

## 3. Goals

### 3.1 MVP goals

The MVP should allow a photographer to do the following.

- Create an account.
- Set up a personal workspace.
- Create photography projects.
- Add goals or shot ideas to a project.
- Create journal entries inside a project.
- Upload images to entries.
- Store images privately.
- Add and manage locations.
- Control location privacy from the start.
- Extract and display basic EXIF metadata.
- Use the app as an installable PWA.
- Create basic offline drafts.
- Sync drafts when back online.
- Use the product confidently as a private creative archive.

### 3.2 Business goals

The MVP should prove whether photographers want a dedicated SaaS product for personal project planning and reflection.

Success means users understand the product quickly, create at least one project, add multiple entries, upload images, and return to continue work.

### 3.3 Technical goals

The MVP should establish a clean SaaS foundation.

- No Firebase dependency.
- Neon Auth for authentication.
- Neon Postgres for application data.
- Cloudflare R2 for image storage.
- Railway for deployment.
- Vite-based frontend.
- API layer between frontend and database.
- PWA foundation.
- Location privacy designed into the data model.
- AI paused until the core product is stable.

## 4. Non-goals for MVP

The following items are explicitly out of scope for the MVP.

- AI tagging.
- AI critique.
- Public project sharing.
- Team accounts.
- Paid subscriptions.
- Native iOS or Android apps.
- Community features.
- Advanced collaboration.
- Full admin console.
- Social feed.
- Marketplace features.
- Client work management.
- Invoicing.
- Contracts.
- CRM functionality.

FotoJournal is not a studio management system. It is for passion projects, personal work, and creative reflection.

## 5. Target users

### 5.1 Amateur photographer

An amateur photographer is a hobby photographer who works on personal themes, travel projects, street photography, family documentary projects, nature projects, or local stories.

Needs include:

- A place to organise project ideas.
- A simple way to track progress.
- Private storage for project notes and images.
- Location planning.
- Memory aids around settings, weather, and decisions.

### 5.2 Professional photographer with personal work

This user is a working photographer who wants a private space for creative work outside client assignments.

Needs include:

- Separation between commercial work and personal work.
- A private creative journal.
- Support for long-running projects.
- Reflection and sequencing tools over time.
- Searchable memory of locations, settings, and ideas.

### 5.3 Serious enthusiast

This user is a photographer developing their craft through personal projects.

Needs include:

- Shot lists.
- Project goals.
- EXIF learning.
- Weather and light planning.
- Visual progress over time.

## 6. Primary user problems

### 6.1 Personal photography projects are scattered

Photographers often track ideas in many places. This makes it hard to keep creative momentum.

### 6.2 Generic note apps do not understand photography

Notes apps can store text and images, but they do not understand EXIF, locations, weather, light, lenses, project goals, or shooting conditions.

### 6.3 Privacy matters

Personal projects often include sensitive locations, private subjects, unfinished work, or images that are not ready to share. Location privacy needs to be built in from the start.

### 6.4 Field use is imperfect

Photographers work outdoors, while travelling, and in locations with poor connectivity. The app should support offline drafts and should not punish users for weak signal.

### 6.5 Creative progress is hard to see

A project may run for weeks, months, or years. Photographers need a way to look back, see what they have done, and decide what comes next.

## 7. Product principles

### 7.1 Privacy first

Images and exact locations are private by default.

### 7.2 Photography-specific

The product should reflect how photographers think, not how generic project management tools think.

### 7.3 Calm and focused

The UI should help users plan and reflect. It should not feel like enterprise software wearing a camera strap.

### 7.4 Useful in the field

The PWA should be installable, fast, and usable with unreliable connectivity.

### 7.5 Simple first

The MVP should solve the core use case before adding AI, teams, billing, or public sharing.

## 8. MVP feature requirements

### 8.1 Authentication and account

Users must be able to create an account, log in, log out, and maintain a session across browser refreshes.

Requirements:

- Use Neon Auth.
- Support email and password signup if Neon Auth supports the desired flow.
- Create an application profile after successful signup.
- Create a personal workspace for each new user.
- Protect all app routes except public auth and marketing pages.
- Persist session across page refresh.
- Allow logout.
- Store app-specific profile data separately from auth internals.

Acceptance criteria:

- A new user can sign up.
- A returning user can log in.
- A logged-in user can refresh the page without losing session.
- A logged-out user cannot access private app routes.
- A new user gets a personal workspace.
- The app does not depend on Firebase Auth.

### 8.2 Personal workspace

Each user gets one personal workspace in the MVP. This prepares the product for future SaaS growth without adding team features yet.

Requirements:

- Create a workspace during onboarding.
- Associate projects, entries, assets, and locations with the workspace.
- Store workspace owner.
- Support future membership model, but expose only a personal workspace in MVP.

Acceptance criteria:

- Every user has one workspace.
- All user-owned data belongs to a workspace.
- API checks workspace ownership before returning or mutating data.

### 8.3 Projects

Projects are the main organising unit in FotoJournal. A project represents a personal photography body of work.

A user can:

- Create a project.
- View a list of projects.
- Open a project detail page.
- Edit project title and description.
- Set project status.
- Archive a project.
- Add an optional theme.
- Add optional start and target end dates.
- Select a cover image after assets exist.

Suggested project statuses:

- Idea.
- Planning.
- Active.
- Paused.
- Completed.
- Archived.

Acceptance criteria:

- User can create a project with title and optional description.
- User sees only their own projects.
- User can update project details.
- User can archive a project.
- Archived projects do not clutter the main view.
- API rejects attempts to access another user's projects.

### 8.4 Project goals and shot ideas

Users should be able to define what they want to achieve or capture within a project.

A user can:

- Add project goals.
- Add shot ideas.
- Mark goals as complete.
- Reorder goals or shot ideas if practical for MVP.
- See open and completed goals on the project page.

Acceptance criteria:

- User can create at least one goal for a project.
- User can mark a goal complete.
- Goals belong to the correct project and workspace.
- Goals help guide future entries.

### 8.5 Journal entries

Entries capture the progress and memory of a photography project. An entry may include notes, images, location, metadata, and future weather information.

A user can:

- Add an entry to a project.
- Add title.
- Add notes.
- Add entry date.
- Upload one or more images, with one image enough for MVP if needed.
- Attach a location.
- View entries on a project page.
- Edit an entry.
- Delete or archive an entry.

Acceptance criteria:

- User can create an entry inside a project.
- User can view entries in reverse chronological order.
- User can edit an entry.
- User can attach at least one image.
- Entry belongs to the correct project and workspace.
- API blocks access to entries outside the user's workspace.

### 8.6 Image upload and storage

Users can upload images to entries. Images are stored privately in Cloudflare R2. Metadata is stored in Neon Postgres.

Requirements:

- Use Cloudflare R2 for image storage.
- Use private bucket by default.
- Generate signed upload URLs through the API.
- Upload directly from browser to R2 using signed URLs.
- Confirm upload with API.
- Store asset metadata in Neon.
- Generate or store image dimensions.
- Store original filename.
- Store MIME type.
- Store byte size.
- Track upload status.
- Provide signed download URLs for private display.
- Avoid permanent public URLs for user images in MVP.

Acceptance criteria:

- User can upload an image to an entry.
- Image is stored in R2.
- Asset metadata is stored in Neon.
- Image can be displayed in the app.
- Failed upload shows a clear error.
- User can retry failed upload.
- Firebase Storage is not used.

### 8.7 Image derivatives

The app should prepare for thumbnails and display versions.

For MVP, create at least a browser-generated thumbnail or store enough metadata to add derivatives later.

Requirements:

- Store original asset record.
- Store derivative records where generated.
- Support thumbnail, preview, and display derivative types.
- Do not block MVP if full derivative processing is deferred.

Acceptance criteria:

- Asset model supports derivatives.
- UI can show an image preview.
- Future server-side processing can be added without redesigning the asset model.

### 8.8 EXIF metadata

FotoJournal should extract and display useful image metadata for photographers.

Extract where available:

- Camera make.
- Camera model.
- Lens model.
- Focal length.
- Aperture.
- Shutter speed.
- ISO.
- Capture timestamp.
- GPS latitude and longitude, if present.

Privacy requirements:

- GPS EXIF must never be exposed publicly by default.
- User must choose whether GPS EXIF should become an entry location.
- Exact GPS data remains private unless explicitly changed by the user.
- Approximate or hidden location settings apply to EXIF-derived location too.

Acceptance criteria:

- Uploaded images show available EXIF data.
- Missing EXIF does not break upload.
- GPS EXIF is private by default.
- User can use EXIF capture date for the entry date.
- EXIF data is stored in Neon.

### 8.9 Location management

Users can add locations to projects and entries, with privacy controls built in from the start.

A user can:

- Select a location on a map.
- Add a label.
- Add optional notes.
- Attach location to project.
- Attach location to entry.
- Choose privacy level.

Privacy levels:

- Exact private.
- Approximate.
- Hidden.

Data handling:

- Store exact coordinates privately as `exact_latitude` and `exact_longitude`.
- Store approximate coordinates separately as `public_latitude` and `public_longitude`.
- Store privacy setting as `privacy_level`.

Acceptance criteria:

- User can attach a location.
- User can set privacy level.
- Exact coordinates are private by default.
- API can return privacy-safe location data.
- Future public sharing cannot accidentally expose exact coordinates.

### 8.10 PWA support

FotoJournal should work as an installable PWA.

Requirements:

- Use Vite.
- Use vite-plugin-pwa.
- Provide web app manifest.
- Provide service worker through Workbox.
- Cache app shell.
- Provide offline fallback page.
- Provide update prompt.
- Provide install prompt where browser allows.
- Avoid hard-coded bundle paths.

Acceptance criteria:

- App can be installed on supported browsers.
- App shell loads offline after first visit.
- User sees a clear offline state.
- Service worker uses generated build assets.
- Manual Create React App service worker is removed.

### 8.11 Offline drafts

The MVP should support basic offline drafting.

Requirements:

- Use IndexedDB through Dexie.js.
- Store draft projects and entries locally.
- Store pending sync operations in an outbox.
- Show pending sync status.
- Retry sync when connection returns.
- Preserve unsynced drafts across refresh.
- Avoid promising full offline image upload in the first MVP unless implementation proves reliable.

Acceptance criteria:

- User can create an entry draft while offline.
- Draft remains after browser refresh.
- Draft syncs when online.
- Failed sync shows a recoverable error.
- User does not lose work silently.

### 8.12 Weather planning

Weather is a should-have feature, not a must-have for the first MVP release.

Future requirements:

- Fetch weather through API, not directly from frontend.
- Keep provider API keys server-side.
- Cache weather responses.
- Store weather snapshots.
- Include sunrise and sunset.
- Include golden hour and blue hour.
- Include cloud cover, rain chance, wind, visibility, and humidity where provider allows.

Acceptance criteria for later release:

- Weather API key is not exposed in browser.
- Weather can be shown for project or entry location.
- Weather snapshots can be attached to entries.
- Golden hour and blue hour are shown for planned shoots.

### 8.13 AI tagging

AI tagging is paused for MVP.

Requirements:

- Remove AI tagging from active entry creation flow.
- Do not load TensorFlow.js or MobileNet in MVP bundle.
- Keep data model ready for future AI-generated tags.
- Support tag source values such as manual, system, and future AI.

Acceptance criteria:

- Entry creation does not depend on AI.
- App bundle is not inflated by unused AI libraries.
- Manual tags remain possible.
- Future AI can be added without reworking tags.

### 8.14 Tags

Users should be able to tag entries manually.

Requirements:

- Add manual tags to entries.
- Store tags per workspace.
- Reuse existing tags.
- Support tag type.

Suggested tag types:

- Manual.
- System.
- Future AI.

Acceptance criteria:

- User can add a tag to an entry.
- User can reuse a tag.
- User can filter or search by tag in a later phase.
- Tag schema supports future AI tags.

## 9. User experience requirements

### 9.1 Onboarding

New users should see:

- A short welcome.
- A prompt to create their first project.
- An optional explanation of private-by-default image and location handling.
- A simple path into the dashboard.

Acceptance criteria:

- New user knows what to do next.
- First project creation is obvious.
- Privacy message is clear without being frightening.

### 9.2 Dashboard

Dashboard should show:

- Active projects.
- Recent entries.
- Quick action to create project.
- Empty state for new users.
- Archived projects accessible separately.

Acceptance criteria:

- User can reach active projects quickly.
- Empty state helps new users create their first project.
- Dashboard does not feel like admin software from 2009.

### 9.3 Project page

Project page should show:

- Project title.
- Description.
- Status.
- Goals or shot ideas.
- Entries.
- Locations if present.
- Cover image if present.
- Add entry action.

Acceptance criteria:

- Project progress is easy to understand.
- Entries are easy to add.
- Goals remain visible while working.

### 9.4 Entry creation

Entry creation should support:

- Title.
- Notes.
- Date.
- Image upload.
- Location.
- Location privacy.
- EXIF preview.
- Save as draft.

Acceptance criteria:

- User can create an entry in under one minute.
- Image upload progress is visible.
- Location privacy is not hidden in obscure settings.
- EXIF data appears when available.

## 10. Technical architecture

### 10.1 Frontend

Use:

- React 19.
- Vite.
- TypeScript 5.
- React Router.
- TanStack Query.
- Dexie.js.
- vite-plugin-pwa.
- Leaflet.
- Zod where helpful.

### 10.2 API

Use:

- Hono.
- Node.js runtime on Railway.
- Drizzle ORM.
- Zod validation.
- Neon Auth session validation.
- Cloudflare R2 signed URLs.
- Structured error responses.
- CORS restricted to known frontend origins.

### 10.3 Database

Use:

- Neon Postgres.
- Drizzle schema and migrations.
- UUID primary keys.
- Workspace ownership.
- Soft deletes where appropriate.
- `created_at` and `updated_at` timestamps.
- Version fields for sync and conflict handling.

### 10.4 Auth

Use:

- Neon Auth.
- App-level profile table linked to Neon Auth user ID.
- Personal workspace created during onboarding.
- Protected API routes.
- Protected frontend routes.

### 10.5 Storage

Use:

- Cloudflare R2.
- Private bucket.
- Signed PUT URLs for upload.
- Signed GET URLs for display.
- Asset metadata in Neon.

### 10.6 Deployment

Use:

- Railway for web service.
- Railway for API service.
- Neon for database.
- Cloudflare for R2.
- Custom domain later.

## 11. Proposed repository structure

```text
apps/
  web/
    React + Vite frontend
  api/
    Hono API
packages/
  db/
    Drizzle schema
    migrations
    database client
  shared/
    Zod schemas
    shared TypeScript types
    API contracts
docs/
  prd.md
  architecture.md
  data-model.md
  deployment.md
  privacy.md
```

## 12. Data model draft

### 12.1 App users

```text
app_users
  id
  neon_auth_user_id
  email
  display_name
  avatar_asset_id
  onboarding_completed_at
  created_at
  updated_at
```

### 12.2 Workspaces

```text
workspaces
  id
  name
  owner_user_id
  plan
  created_at
  updated_at
```

### 12.3 Workspace members

```text
workspace_members
  id
  workspace_id
  user_id
  role
  created_at
```

### 12.4 Projects

```text
projects
  id
  workspace_id
  owner_user_id
  title
  description
  status
  theme
  start_date
  target_end_date
  cover_asset_id
  default_location_privacy
  created_at
  updated_at
  archived_at
  deleted_at
```

### 12.5 Project goals

```text
project_goals
  id
  project_id
  title
  notes
  sort_order
  completed_at
  created_at
  updated_at
```

### 12.6 Entries

```text
entries
  id
  project_id
  workspace_id
  created_by_user_id
  title
  notes
  entry_date
  location_id
  weather_snapshot_id
  mood
  status
  created_at
  updated_at
  deleted_at
```

### 12.7 Locations

```text
locations
  id
  workspace_id
  created_by_user_id
  label
  exact_latitude
  exact_longitude
  public_latitude
  public_longitude
  privacy_level
  notes
  created_at
  updated_at
```

### 12.8 Assets

```text
assets
  id
  workspace_id
  uploaded_by_user_id
  project_id
  entry_id
  storage_provider
  bucket
  object_key
  original_filename
  mime_type
  byte_size
  width
  height
  checksum_sha256
  upload_status
  visibility
  created_at
  updated_at
  deleted_at
```

### 12.9 Asset derivatives

```text
asset_derivatives
  id
  asset_id
  kind
  object_key
  width
  height
  byte_size
  created_at
```

### 12.10 EXIF metadata

```text
exif_metadata
  asset_id
  camera_make
  camera_model
  lens_model
  focal_length
  aperture
  shutter_speed
  iso
  captured_at
  gps_latitude
  gps_longitude
  gps_privacy_applied
  raw_json
```

### 12.11 Tags

```text
tags
  id
  workspace_id
  name
  type
  created_at
```

### 12.12 Entry tags

```text
entry_tags
  entry_id
  tag_id
```

### 12.13 Weather snapshots

```text
weather_snapshots
  id
  provider
  latitude
  longitude
  observed_at
  condition
  temperature_c
  humidity
  cloud_cover
  wind_speed
  visibility
  sunrise_at
  sunset_at
  golden_hour_start
  golden_hour_end
  blue_hour_start
  blue_hour_end
  raw_json
  created_at
```

### 12.14 Sync events

```text
sync_events
  id
  workspace_id
  user_id
  client_id
  entity_type
  entity_id
  operation
  payload
  status
  created_at
  processed_at
```

## 13. API requirements

### 13.1 Health

```text
GET /health
```

Returns API health state.

### 13.2 Current user

```text
GET /api/me
```

Returns authenticated user, profile, and workspace.

### 13.3 Onboarding

```text
POST /api/onboarding
```

Creates app profile and personal workspace if needed.

### 13.4 Projects

```text
GET    /api/projects
POST   /api/projects
GET    /api/projects/:id
PATCH  /api/projects/:id
DELETE /api/projects/:id
```

### 13.5 Project goals

```text
GET    /api/projects/:projectId/goals
POST   /api/projects/:projectId/goals
PATCH  /api/project-goals/:id
DELETE /api/project-goals/:id
```

### 13.6 Entries

```text
GET    /api/projects/:projectId/entries
POST   /api/projects/:projectId/entries
GET    /api/entries/:id
PATCH  /api/entries/:id
DELETE /api/entries/:id
```

### 13.7 Locations

```text
POST  /api/locations
GET   /api/locations/:id
PATCH /api/locations/:id
```

### 13.8 Assets

```text
POST /api/assets/upload-request
POST /api/assets/upload-complete
GET  /api/assets/:id/download-url
```

### 13.9 Sync

```text
POST /api/sync
GET  /api/sync/status
```

## 14. Security and privacy requirements

### 14.1 Authentication

- All private API routes require valid Neon Auth session.
- Session validation must happen server-side.
- API must not trust user ID from client payload.
- API must derive user and workspace from session.

### 14.2 Authorization

- All project, entry, asset, location, and tag access must check workspace membership.
- MVP supports personal workspace only.
- Future team support should not require a data model rewrite.

### 14.3 Location privacy

- Exact coordinates are private by default.
- Public-safe coordinates are stored separately.
- Hidden locations return no coordinates in public-safe responses.
- EXIF GPS must follow the same privacy rules.
- Future public sharing must never expose exact coordinates by accident.

### 14.4 Image privacy

- User images are private by default.
- R2 bucket should not expose all objects publicly.
- Use signed URLs for upload and display.
- Signed URLs should expire.
- API should verify ownership before issuing signed download URLs.

### 14.5 Environment secrets

- Secrets must be stored in Railway and Cloudflare dashboards.
- No secrets in source control.
- Frontend environment variables must not contain private API keys.

## 15. Performance requirements

### 15.1 Frontend

- Initial app load should be fast enough for mobile use.
- AI libraries should not ship in MVP bundle.
- Image previews should be sized appropriately.
- PWA app shell should load quickly after first visit.

### 15.2 API

- Common project and entry endpoints should respond quickly under normal MVP load.
- Database queries should be indexed by workspace, project, and user where needed.
- R2 upload flow should avoid proxying large image files through the API.

### 15.3 Images

- Upload directly from browser to R2.
- Store image dimensions.
- Generate thumbnails where possible.
- Avoid loading original full-size images in list views.

## 16. Offline requirements

### 16.1 MVP offline scope

MVP should support:

- Offline app shell.
- Offline draft entries.
- Offline draft project notes if feasible.
- Pending sync queue.
- Clear sync status.

MVP does not need to guarantee perfect large-image offline upload across all browsers.

### 16.2 Outbox pattern

The app should store pending operations locally.

```text
outbox
  id
  operation
  entity_type
  entity_id
  payload
  status
  created_at
  retry_count
  last_error
```

### 16.3 Acceptance criteria

- User can create a draft offline.
- Draft remains after refresh.
- Draft syncs when online.
- User can see pending state.
- Failed sync can be retried.

## 17. MVP user journeys

### 17.1 New user creates first project

1. User signs up.
2. App creates profile and personal workspace.
3. User lands on dashboard.
4. Empty state prompts user to create a project.
5. User adds project title and description.
6. Project appears on dashboard.

### 17.2 User adds an entry with image

1. User opens a project.
2. User selects Add Entry.
3. User enters title, notes, and date.
4. User uploads image.
5. App extracts EXIF.
6. User reviews EXIF.
7. User optionally selects location and privacy level.
8. User saves entry.
9. Entry appears on project page.

### 17.3 User captures a location privately

1. User opens map picker.
2. User selects exact location.
3. App sets privacy to exact private by default.
4. User may change to approximate or hidden.
5. API stores exact and public-safe coordinates separately.
6. UI labels the privacy state clearly.

### 17.4 User creates a draft offline

1. User opens installed PWA with weak or no connectivity.
2. User creates an entry draft.
3. App saves draft locally.
4. App shows pending sync status.
5. User reconnects.
6. App syncs draft to API.
7. Draft becomes normal entry.

## 18. MVP success metrics

### 18.1 Activation

- Percentage of users who create a project within their first session.
- Percentage of users who add an entry within their first session.
- Percentage of users who upload at least one image.

### 18.2 Engagement

- Number of projects per active user.
- Number of entries per project.
- Return rate after seven days.
- Number of images uploaded per user.

### 18.3 Product value

- Percentage of users adding project goals.
- Percentage of entries with images.
- Percentage of entries with location.
- Percentage of uploaded images with EXIF displayed.

### 18.4 Reliability

- Upload failure rate.
- API error rate.
- Sync failure rate.
- Time to first meaningful screen.
- PWA install rate.

## 19. Release plan

### 19.1 Phase 1, foundation

- Repo restructure.
- Vite frontend.
- PWA foundation.
- Neon and Drizzle.
- Hono API.
- Railway deployment skeleton.

### 19.2 Phase 2, auth and workspace

- Neon Auth integration.
- Session handling.
- User profile.
- Personal workspace.
- Protected routes.

### 19.3 Phase 3, core product

- Projects.
- Project goals.
- Entries.
- Tags.
- Dashboard.
- Project detail page.

### 19.4 Phase 4, images and privacy

- Cloudflare R2 uploads.
- Asset metadata.
- Signed URLs.
- Location privacy.
- EXIF extraction.

### 19.5 Phase 5, offline MVP

- IndexedDB drafts.
- Outbox.
- Sync status.
- Basic retry.

### 19.6 Phase 6, polish and release hardening

- Error handling.
- Empty states.
- Tests.
- Railway production deployment.
- Privacy page.
- Terms page.
- MVP release checklist.

## 20. Suggested implementation PR sequence

### 20.1 PR 1, workspace and Vite foundation

- Create monorepo layout.
- Move web app into `apps/web`.
- Replace Create React App with Vite.
- Upgrade TypeScript.
- Remove Firebase runtime assumptions from app shell.
- Pause active AI tagging.

### 20.2 PR 2, PWA foundation

- Add vite-plugin-pwa.
- Replace manual service worker.
- Add manifest configuration.
- Add offline fallback.
- Add update prompt.

### 20.3 PR 3, database package

- Add Drizzle.
- Add Neon connection.
- Add schema.
- Add migrations.
- Add seed script.

### 20.4 PR 4, API skeleton

- Add Hono API.
- Add health route.
- Add environment validation.
- Add database client.
- Add Railway-ready scripts.

### 20.5 PR 5, Neon Auth

- Add Neon Auth integration.
- Add session handling.
- Add protected routes.
- Add onboarding profile and workspace creation.

### 20.6 PR 6, projects and entries

- Add project API routes.
- Add entry API routes.
- Add frontend query and mutation hooks.
- Replace Firestore flows.

### 20.7 PR 7, location privacy

- Add location model.
- Add exact, approximate, and hidden handling.
- Add UI controls.
- Add API privacy tests.

### 20.8 PR 8, R2 uploads

- Add signed upload URL route.
- Add upload complete route.
- Add asset records.
- Add frontend upload flow.
- Remove Firebase Storage usage.

### 20.9 PR 9, offline drafts and sync

- Add Dexie.
- Add outbox.
- Add offline draft support.
- Add sync state UI.

### 20.10 PR 10, EXIF and photography polish

- Add EXIF extraction.
- Add EXIF display.
- Add basic shot goals.
- Improve entry workflow.

## 21. Risks and mitigations

### 21.1 Neon Auth beta risk

Risk:

Neon Auth is currently beta, so integration details may change or production readiness may require validation.

Mitigation:

Build a proof of concept early. Confirm signup, login, session persistence, Railway deployment, and API validation before building all product flows on top.

### 21.2 Offline complexity

Risk:

Replacing Firestore offline behaviour requires custom sync logic.

Mitigation:

Limit MVP offline scope to drafts and basic outbox sync. Avoid promising full offline image upload until tested.

### 21.3 Image upload reliability

Risk:

Large photo uploads can fail on mobile networks.

Mitigation:

Use direct-to-R2 signed uploads, progress indicators, retry states, and clear error messages.

### 21.4 Location privacy mistakes

Risk:

Exact coordinates could be exposed accidentally in future public sharing.

Mitigation:

Store exact and public-safe coordinates separately. Ensure API response shapes enforce privacy.

### 21.5 Scope creep

Risk:

The SaaS MVP grows into teams, AI, billing, community, and native apps before the core problem is validated.

Mitigation:

Keep MVP focused on private personal photography projects. Defer AI, billing, public sharing, and teams.

## 22. Open questions

- Should MVP include a marketing landing page, or only the authenticated app?
- Should weather planning be in the first release or the second?
- Should users be able to export a project in MVP?
- Should image upload support multiple images per entry in MVP, or start with one?
- Should project goals and shot lists be separate concepts, or one shared list for MVP?
- Should there be a free beta before paid plans are added?
- Which custom domain will be used for the app?
- Will the product name remain FotoJournal?

## 23. Recommended MVP cut

The recommended first public MVP should include:

- Neon Auth signup and login.
- Personal workspace.
- Project creation and management.
- Project goals.
- Journal entries.
- Private image upload to R2.
- EXIF extraction and display.
- Location selection.
- Location privacy.
- Basic manual tags.
- PWA installability.
- Offline draft entries.
- Railway deployment.
- Neon Postgres database.
- Clean SaaS-ready architecture.

Defer:

- AI tagging.
- Billing.
- Public sharing.
- Team workspaces.
- Advanced weather planning.
- Native mobile apps.

## 24. Definition of done for MVP

The MVP is done when:

- A new user can sign up and create a personal workspace.
- A user can create a project.
- A user can add project goals.
- A user can create an entry.
- A user can upload an image privately.
- EXIF metadata appears when available.
- A user can add a private, approximate, or hidden location.
- A user can install the PWA.
- A user can create an offline draft and sync it later.
- API authorization prevents cross-user data access.
- App deploys successfully on Railway.
- Data is stored in Neon Postgres.
- Images are stored in Cloudflare R2.
- Firebase is no longer part of the runtime architecture.
- AI tagging is not active in the MVP bundle.
- Core flows are covered by automated tests.
- Privacy behaviour is documented.

## 25. Final recommendation

Build the MVP around privacy, personal projects, image journaling, EXIF, and location planning.

Do not start with AI. Do not start with teams. Do not start with billing. Those can come later, after the product proves that photographers want a dedicated home for personal creative projects.

The right first product is a focused, private, installable photography project journal that feels like it was made by someone who understands why a photographer might return to the same corner of a city at 6:12am because the light finally behaves itself.
