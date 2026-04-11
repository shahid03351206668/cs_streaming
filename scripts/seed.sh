#!/usr/bin/env bash
# =============================================================================
# seed.sh — Populate the Tasksy server with realistic test data
#
# Creates:
#   • 1 Admin user
#   • 10 Client users
#   • 10 Freelancer users
#   • 8 Job categories
#   • 40 Job posts (4 per client, mixed statuses & locations)
#   • 60 Proposals (freelancers bid on open jobs)
#   • 20 Contracts (accepted proposals → contracts)
#   • 8  Disputes  (filed on active contracts, ~half auto-resolved)
#
# Usage:
#   chmod +x scripts/seed.sh
#   ./scripts/seed.sh                        # default: http://localhost:8080
#   BASE_URL=http://myserver:8080 ./scripts/seed.sh
#
# Requirements: curl, jq
# =============================================================================

set -uo pipefail   # NOTE: intentionally no -e; arithmetic counters start at 0

BASE_URL="${BASE_URL:-http://localhost:8080}"
ADMIN_EMAIL="admin@tasksy.dev"
ADMIN_PASS="Admin@1234"
SEED_PASS="Test@1234"

# ── Colours ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

info()    { echo -e "${CYAN}[INFO]${RESET}  $*"; }
success() { echo -e "${GREEN}[OK]${RESET}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${RESET}  $*"; }
error()   { echo -e "${RED}[ERROR]${RESET} $*" >&2; }
section() {
  echo -e "\n${BOLD}══════════════════════════════════════════${RESET}"
  echo -e "${BOLD} $*${RESET}"
  echo -e "${BOLD}══════════════════════════════════════════${RESET}"
}

# ── Requirements ─────────────────────────────────────────────────────────────
for cmd in curl jq; do
  command -v "$cmd" >/dev/null 2>&1 || { error "Required command '$cmd' not found."; exit 1; }
done

# ── HTTP helpers ─────────────────────────────────────────────────────────────
# post <path> <json> [token]
post() {
  local path="$1" body="$2" token="${3:-}"
  if [[ -n "$token" ]]; then
    curl -s -X POST -H "Content-Type: application/json" \
      -H "Authorization: Bearer $token" -d "$body" "${BASE_URL}${path}"
  else
    curl -s -X POST -H "Content-Type: application/json" -d "$body" "${BASE_URL}${path}"
  fi
}

# put <path> <json> [token]
put() {
  local path="$1" body="$2" token="${3:-}"
  if [[ -n "$token" ]]; then
    curl -s -X PUT -H "Content-Type: application/json" \
      -H "Authorization: Bearer $token" -d "$body" "${BASE_URL}${path}"
  else
    curl -s -X PUT -H "Content-Type: application/json" -d "$body" "${BASE_URL}${path}"
  fi
}

# login <email> <password>  → prints access token
login() {
  local resp
  resp=$(post "/api/auth/login" "{\"email\":\"$1\",\"password\":\"$2\"}")
  echo "$resp" | jq -r '.tokens.access_token // empty'
}

# register <first> <last> <email> <pass>  → prints user id
register() {
  local resp
  resp=$(curl -s -X POST \
    -F "first_name=$1" -F "last_name=$2" -F "email=$3" -F "password=$4" \
    "${BASE_URL}/api/auth/register")
  echo "$resp" | jq -r '.user.id // empty'
}

# get_my_id <token>  → prints user id from /api/user/profile
get_my_id() {
  curl -s -H "Authorization: Bearer $1" "${BASE_URL}/api/user/profile" \
    | jq -r '.user.id // empty'
}

# verify_user <uid> <admin_token>
verify_user() {
  [[ -z "$1" ]] && return
  put "/api/v1/admin/users/$1" '{"identity_verified":true,"email_verified":true}' "$2" >/dev/null
}

# rand_int <min> <max>
rand_int() { echo $(( RANDOM % ($2 - $1 + 1) + $1 )); }

# rand_letter  → random A-Z letter
rand_letter() {
  local letters=(A B C D E F G H I J K L M N O P Q R S T U V W X Y Z)
  echo "${letters[$(( RANDOM % 26 ))]}"
}

# ── Server check ─────────────────────────────────────────────────────────────
section "Checking server connectivity"
ping_resp=$(curl -s "${BASE_URL}/ping" 2>/dev/null || true)
if [[ "$(echo "$ping_resp" | jq -r '.message // empty' 2>/dev/null)" != "pong" ]]; then
  error "Server at ${BASE_URL} is not responding. Start it first."
  exit 1
fi
success "Server is up at ${BASE_URL}"

# =============================================================================
# 1. ADMIN
# =============================================================================
section "1 · Admin user"

# Try register first (idempotent — 409 on second run is fine)
curl -s -X POST \
  -F "first_name=Super" -F "last_name=Admin" \
  -F "email=${ADMIN_EMAIL}" -F "password=${ADMIN_PASS}" \
  "${BASE_URL}/api/auth/register" >/dev/null || true

ADMIN_TOKEN=$(login "$ADMIN_EMAIL" "$ADMIN_PASS")
if [[ -z "$ADMIN_TOKEN" ]]; then
  error "Cannot obtain admin token. Check server logs."
  exit 1
fi
success "Admin token obtained  (${ADMIN_EMAIL})"

# =============================================================================
# 2. CATEGORIES
# =============================================================================
section "2 · Categories"

CAT_NAMES=(
  "Web Development" "Mobile Development" "Graphic Design"
  "Content Writing" "Digital Marketing"  "Video Editing"
  "Data Science"    "Home Repairs"
)
VALID_CATS=()

for cat_name in "${CAT_NAMES[@]}"; do
  resp=$(post "/api/v1/category/create" "{\"name\":\"${cat_name}\"}" "$ADMIN_TOKEN")
  cat_id=$(echo "$resp" | jq -r '.data.id // .id // empty')
  if [[ -z "$cat_id" ]]; then
    # already exists — fetch from list
    cat_id=$(curl -s "${BASE_URL}/api/v1/category/list" \
      | jq -r --arg n "$cat_name" '(.data // .) | .[] | select(.name==$n) | .id' 2>/dev/null \
      | head -1)
  fi
  if [[ -n "$cat_id" ]]; then
    VALID_CATS+=("$cat_id")
    success "Category: ${cat_name} → ${cat_id}"
  else
    warn "Could not create/find category: ${cat_name}"
  fi
done

if [[ ${#VALID_CATS[@]} -eq 0 ]]; then
  error "No categories available. Cannot continue."
  exit 1
fi
info "Valid categories: ${#VALID_CATS[@]}"

# =============================================================================
# 3. CLIENT USERS (10)
# =============================================================================
section "3 · Client users (10)"

CLIENT_FIRST=(Alice   Bob    Carol   David  Emma   Frank  Grace   Henry   Isabel  James)
CLIENT_LAST=( Martin  Chen   Okafor  Patel  Silva  Muller Nguyen  Stewart Rossi   Taylor)
CLIENT_TOKENS=()
CLIENT_IDS=()

for i in "${!CLIENT_FIRST[@]}"; do
  first="${CLIENT_FIRST[$i]}"
  last="${CLIENT_LAST[$i]}"
  email_key=$(echo "${first}${last}" | tr '[:upper:]' '[:lower:]')
  email="client.${email_key}@seed.tasksy.dev"

  uid=$(register "$first" "$last" "$email" "$SEED_PASS")
  token=$(login "$email" "$SEED_PASS")

  if [[ -z "$token" ]]; then
    warn "Skipping client ${email} — login failed"
    CLIENT_IDS+=("")
    CLIENT_TOKENS+=("")
    continue
  fi

  [[ -z "$uid" ]] && uid=$(get_my_id "$token")

  verify_user "$uid" "$ADMIN_TOKEN"
  CLIENT_IDS+=("$uid")
  CLIENT_TOKENS+=("$token")
  success "Client $((i+1)): ${first} ${last} (${uid:-NO-ID})"
done

# =============================================================================
# 4. FREELANCER USERS (10)
# =============================================================================
section "4 · Freelancer users (10)"

FL_FIRST=(Liam   Mia    Noah   Olivia Ethan  Sophie Lucas  Amara  Ryan   Zara)
FL_LAST=( Walker Brown  Davis  Wilson Moore  Garcia Thomas Jones  White  Harris)
FREELANCER_TOKENS=()
FREELANCER_IDS=()

for i in "${!FL_FIRST[@]}"; do
  first="${FL_FIRST[$i]}"
  last="${FL_LAST[$i]}"
  email_key=$(echo "${first}${last}" | tr '[:upper:]' '[:lower:]')
  email="freelancer.${email_key}@seed.tasksy.dev"

  uid=$(register "$first" "$last" "$email" "$SEED_PASS")
  token=$(login "$email" "$SEED_PASS")

  if [[ -z "$token" ]]; then
    warn "Skipping freelancer ${email} — login failed"
    FREELANCER_IDS+=("")
    FREELANCER_TOKENS+=("")
    continue
  fi

  [[ -z "$uid" ]] && uid=$(get_my_id "$token")

  verify_user "$uid" "$ADMIN_TOKEN"
  FREELANCER_IDS+=("$uid")
  FREELANCER_TOKENS+=("$token")
  success "Freelancer $((i+1)): ${first} ${last} (${uid:-NO-ID})"
done

# =============================================================================
# 5. JOB POSTS (4 per client = up to 40)
# =============================================================================
section "5 · Job posts (4 per client)"

JOB_TITLES=(
  "Build a responsive e-commerce website with React"
  "Develop a REST API backend in Go/Node.js"
  "Design a modern mobile app UI/UX in Figma"
  "Write SEO-optimised blog content (10 articles)"
  "Create a cross-platform iOS/Android app"
  "Produce a 2-minute product explainer video"
  "Set up Google Ads and Facebook Ads campaigns"
  "Analyse sales data and build BI dashboards"
  "Build a real-time chat application"
  "Design a company brand identity package"
  "Write technical documentation for REST API"
  "Develop a WordPress plugin for membership"
  "Fix plumbing issues in a 3-bedroom house"
  "Paint the exterior of a residential property"
  "Create automated social media content calendar"
  "Build a machine learning price prediction model"
  "Develop a Flutter mobile app for food delivery"
  "Write 5 white-papers on blockchain technology"
  "Set up CI/CD pipelines with GitHub Actions"
  "Design and print marketing brochures"
  "Build a React Native fitness tracking app"
  "Develop a Node.js microservices architecture"
  "Create motion graphics for YouTube channel"
  "Write product descriptions for 200 SKUs"
  "Set up Kubernetes cluster on AWS EKS"
  "Train a custom NLP text classification model"
  "Build an admin dashboard with Next.js"
  "Edit and produce 10 podcast episodes"
  "Repair and refurbish kitchen cabinets"
  "Design a SaaS landing page with animations"
  "Migrate legacy PHP app to modern Laravel"
  "Create a full-stack appointment booking system"
  "Produce a corporate photography shoot"
  "Build an automated email marketing funnel"
  "Develop IoT sensor data collection system"
  "Create a mobile game in Unity"
  "Write copy for a crowdfunding campaign"
  "Set up Shopify store with custom theme"
  "Fix electrical wiring in a commercial unit"
  "Build a real-time data pipeline with Kafka"
)

JOB_DESCS=(
  "I need a fully responsive e-commerce site with product listings, cart, checkout, and Stripe integration. Must work on mobile and desktop. Deliverables include source code and Vercel deployment."
  "Need a scalable REST API with JWT authentication, PostgreSQL database, and full CRUD for products and users. Include Swagger docs and Docker setup."
  "Looking for an experienced UI/UX designer to create wireframes and high-fidelity mockups for a FinTech mobile app following modern design trends, with a complete design system."
  "Require 10 well-researched SEO-optimised blog articles on digital marketing topics. Each 1500+ words with proper headings, meta descriptions, and internal linking suggestions."
  "Build a cross-platform app using React Native for iOS and Android with user auth, push notifications, offline support, and a social feed feature."
  "Create a professional 2-minute animated explainer video for our SaaS product. Provide script, voiceover, animation, and final exported MP4."
  "Set up and manage Google Ads and Facebook ad campaigns for our e-commerce store. Target 4x ROAS with monthly reporting and continuous optimisation."
  "Analyse 3 years of sales data, identify trends, and build interactive dashboards in Power BI or Tableau. Deliver insights report and forecasting model."
  "Build a real-time chat app using WebSockets with rooms, direct messages, read receipts, file sharing, and full message history."
  "Design a complete brand identity including logo, colour palette, typography, business cards, letterhead, and brand guidelines PDF."
  "Write comprehensive technical documentation for our REST API including authentication guide, endpoint reference, code samples in 3 languages, and error reference."
  "Develop a WordPress plugin for membership subscriptions, content gating, Stripe billing, and a member directory."
  "Fix plumbing issues including a leaking kitchen sink, blocked bathroom drain, and faulty shower valve. Must be a certified plumber."
  "Paint the exterior of a 4-bedroom detached house including surface prep, primer, and two finishing coats. Provide all materials."
  "Plan, create, and schedule 30 days of social media content for Instagram, Twitter, and LinkedIn including graphics, captions, and hashtags."
  "Build a machine learning model to predict product prices using historical data. Use Python and scikit-learn, deploy as a FastAPI endpoint."
  "Develop a Flutter food delivery app with restaurant listings, menu browsing, cart, order tracking, and push notifications. Backend already exists."
  "Write 5 in-depth white-papers of 3000+ words each on blockchain applications in finance, supply chain, healthcare, gaming, and identity."
  "Set up end-to-end CI/CD pipelines using GitHub Actions for a Node.js monorepo including linting, testing, staging deploys, and production releases."
  "Design and arrange printing of A4 and A5 marketing brochures for our product launch. Provide print-ready PDF files in CMYK."
  "Build a React Native fitness tracking app with workout logging, exercise library, progress charts, and Apple Health/Google Fit integration."
  "Architect and implement a Node.js microservices system with an API gateway, auth service, product service, and order service using Docker."
  "Create custom motion graphics and animated transitions for a YouTube tech channel. Deliver After Effects project files and rendered MP4s."
  "Write engaging SEO-friendly product descriptions for 200 products in our online store. Match our tone of voice guidelines."
  "Set up a production-ready Kubernetes cluster on AWS EKS with Helm charts, autoscaling, and Prometheus/Grafana monitoring."
  "Train a custom NLP model to classify support tickets and predict urgency using HuggingFace Transformers. Include evaluation metrics."
  "Build a full-featured admin dashboard using Next.js 14, Tailwind CSS, and shadcn/ui with users, analytics, and settings modules."
  "Edit 10 podcast episodes including noise removal, levelling, music intro/outro, chapter markers, and export in multiple formats."
  "Repair and refurbish 20 kitchen cabinet doors: sand, fill, repaint in matte white, and replace hinges and handles."
  "Design a high-converting SaaS landing page with smooth scroll animations, pricing table, testimonials section, and mobile responsiveness."
  "Migrate a legacy PHP 5.6 codebase to modern Laravel 11 with proper MVC structure, unit tests, and CI pipeline."
  "Build a full-stack appointment booking system with calendar integration, automated email reminders, and an admin management dashboard."
  "Produce a half-day corporate photography session for our team page. Deliver 50 edited high-res headshots and 20 office shots."
  "Build a Klaviyo/Mailchimp automated email funnel: welcome series, abandoned cart, post-purchase, and win-back sequences."
  "Develop an IoT data collection system that receives sensor data via MQTT, stores it in InfluxDB, and visualises in Grafana."
  "Create a casual mobile game in Unity for iOS and Android with 3 levels, ad monetisation, and a leaderboard."
  "Write compelling crowdfunding campaign copy for Kickstarter including the main page, reward descriptions, and FAQs."
  "Set up a complete Shopify store with a custom theme, product imports, payment gateway, shipping rules, and analytics."
  "Diagnose and fix electrical wiring faults in a 1500 sq ft commercial unit. Must be a qualified electrician."
  "Build a real-time data pipeline using Apache Kafka and PostgreSQL to process and aggregate streaming event data from a mobile app."
)

JOB_BUDGETS=(
  1500 1200 800  500  2500 1000 1500 2000 1800 700
  600  1400 400  800  600  2500 2000 1500 900  500
  2200 1800 800  900  2800 3000 1600 700  600  1200
  2000 2200 1000 1400 2500 2000 800  1200 600  2800
)

JOB_CITIES=(
  London     Manchester Birmingham Leeds      Glasgow
  Bristol    Sheffield  Liverpool  Edinburgh  Cardiff
  London     Manchester Birmingham Leeds      Glasgow
  Bristol    Sheffield  Liverpool  Edinburgh  Cardiff
  London     Manchester Birmingham Leeds      Glasgow
  Bristol    Sheffield  Liverpool  Edinburgh  Cardiff
  London     Manchester Birmingham Leeds      Glasgow
  Bristol    Sheffield  Liverpool  Edinburgh  Cardiff
)

JOB_LATS=(
  51.5074 53.4808 52.4862 53.8008 55.8642
  51.4545 53.3811 53.4084 55.9533 51.4816
  51.5074 53.4808 52.4862 53.8008 55.8642
  51.4545 53.3811 53.4084 55.9533 51.4816
  51.5074 53.4808 52.4862 53.8008 55.8642
  51.4545 53.3811 53.4084 55.9533 51.4816
  51.5074 53.4808 52.4862 53.8008 55.8642
  51.4545 53.3811 53.4084 55.9533 51.4816
)

JOB_LONS=(
  -0.1278 -2.2426 -1.8904 -1.5491 -4.2518
  -2.5879 -1.4701 -2.9916 -3.1883 -3.1791
  -0.1278 -2.2426 -1.8904 -1.5491 -4.2518
  -2.5879 -1.4701 -2.9916 -3.1883 -3.1791
  -0.1278 -2.2426 -1.8904 -1.5491 -4.2518
  -2.5879 -1.4701 -2.9916 -3.1883 -3.1791
  -0.1278 -2.2426 -1.8904 -1.5491 -4.2518
  -2.5879 -1.4701 -2.9916 -3.1883 -3.1791
)

JOB_IDS=()
JOB_CLIENT_IDX=()
NUM_CATS=${#VALID_CATS[@]}
job_num=0   # absolute index across all jobs

for client_idx in "${!CLIENT_TOKENS[@]}"; do
  ctoken="${CLIENT_TOKENS[$client_idx]}"
  [[ -z "$ctoken" ]] && continue

  for j in 0 1 2 3; do
    [[ $job_num -ge ${#JOB_TITLES[@]} ]] && break

    title="${JOB_TITLES[$job_num]}"
    desc="${JOB_DESCS[$job_num]}"
    budget="${JOB_BUDGETS[$job_num]}"
    city="${JOB_CITIES[$job_num]}"
    lat="${JOB_LATS[$job_num]}"
    lon="${JOB_LONS[$job_num]}"
    cat_id="${VALID_CATS[$(( job_num % NUM_CATS ))]}"

    # Generate a plausible UK postcode
    prefix=$(echo "${city:0:2}" | tr '[:lower:]' '[:upper:]')
    postcode="${prefix}$(rand_int 1 9) $(rand_int 1 9)$(rand_letter)$(rand_letter)"

    resp=$(curl -s -X POST \
      -H "Authorization: Bearer ${ctoken}" \
      -F "title=${title}" \
      -F "description=${desc}" \
      -F "budget=${budget}" \
      -F "open_budget=false" \
      -F "category_id=${cat_id}" \
      -F "address=${city}, United Kingdom" \
      -F "city=${city}" \
      -F "state=England" \
      -F "country=United Kingdom" \
      -F "latitude=${lat}" \
      -F "longitude=${lon}" \
      -F "postalcode=${postcode}" \
      -F "street=$(rand_int 1 200) High Street" \
      "${BASE_URL}/api/job/create")

    job_id=$(echo "$resp" | jq -r '.data.id // empty')
    if [[ -n "$job_id" ]]; then
      JOB_IDS+=("$job_id")
      JOB_CLIENT_IDX+=("$client_idx")
      success "Job $((job_num+1)): \"${title:0:48}...\" (${job_id})"
    else
      err_msg=$(echo "$resp" | jq -r '.error // .message // "unknown"')
      warn "Job $((job_num+1)) failed: ${err_msg}"
      JOB_IDS+=("")
      JOB_CLIENT_IDX+=("")
    fi

    job_num=$(( job_num + 1 ))
  done
done

info "Total jobs created: $(printf '%s\n' "${JOB_IDS[@]}" | grep -c . || true)"

# =============================================================================
# 6. PROPOSALS (~6 bids per freelancer)
# =============================================================================
section "6 · Proposals"

COVER_LETTERS=(
  "I have 5+ years of experience in this area and have delivered similar projects for clients across Europe. My approach is iterative with weekly check-ins. I am confident I can deliver on time and within budget."
  "Having reviewed your project brief carefully, I believe my skill set is a perfect match. I recently completed a very similar engagement and can share the case study on request. Let me take this off your plate."
  "Your project aligns exactly with my core expertise. I hold relevant certifications and follow industry best practices. I'll start with a discovery call to align on requirements, then provide a detailed project plan before writing a single line of code."
  "I've read your description thoroughly and have great ideas to enhance the outcome beyond what you've asked for. I'm available to start immediately and can dedicate 30+ hours per week to this project until completion."
  "Quality and communication are my top priorities. I will provide daily status updates, share work-in-progress for early feedback, and ensure zero surprises at delivery. My past clients consistently rate me 5 stars."
  "This project matches what I do day in and day out. I can bring not only technical execution but strategic thinking to help you achieve a better result. Happy to do a no-obligation 30-minute intro call first."
  "I specialise in exactly this type of work with 50+ successful projects. I use agile methodology which means you get working deliverables early and often, reducing risk significantly."
  "After carefully analysing your requirements, I'm confident this is well within my abilities. I have a structured process: kick-off, design sprint, development, QA, delivery. You'll never wonder where things stand."
  "My portfolio includes several projects very close to what you're describing. I understand the nuances involved and have the tools to handle edge cases that trip up less experienced contractors."
  "I'm excited about this project because it combines two areas I'm passionate about. I'll go above and beyond to make sure you're delighted, and I stand behind my work with a revision guarantee."
)

PROPOSAL_IDS=()
PROPOSAL_JOB_IDX=()
PROPOSAL_FL_IDX=()
proposal_count=0
declare -A bid_map=()

for fl_idx in "${!FREELANCER_TOKENS[@]}"; do
  fl_token="${FREELANCER_TOKENS[$fl_idx]}"
  [[ -z "$fl_token" ]] && continue

  bids_this_fl=0
  for job_arr_idx in "${!JOB_IDS[@]}"; do
    [[ $bids_this_fl -ge 6 ]] && break
    job_id="${JOB_IDS[$job_arr_idx]}"
    [[ -z "$job_id" ]] && continue

    bid_key="${job_id}:${fl_idx}"
    [[ -n "${bid_map[$bid_key]+_}" ]] && continue
    bid_map["$bid_key"]=1

    cover="${COVER_LETTERS[$(( proposal_count % ${#COVER_LETTERS[@]} ))]}"
    budget="${JOB_BUDGETS[$job_arr_idx]:-1000}"
    # Bid 80–110 % of budget using pure bash arithmetic
    pct=$(rand_int 80 110)
    bid=$(( budget * pct / 100 ))
    duration=$(rand_int 5 25)

    resp=$(curl -s -X POST \
      -H "Authorization: Bearer ${fl_token}" \
      -F "job_post_id=${job_id}" \
      -F "cover_letter=${cover}" \
      -F "bid_amount=${bid}" \
      -F "duration=${duration}" \
      "${BASE_URL}/api/job/send-proposal")

    prop_id=$(echo "$resp" | jq -r '.data.id // empty')
    if [[ -n "$prop_id" ]]; then
      PROPOSAL_IDS+=("$prop_id")
      PROPOSAL_JOB_IDX+=("$job_arr_idx")
      PROPOSAL_FL_IDX+=("$fl_idx")
      proposal_count=$(( proposal_count + 1 ))
      bids_this_fl=$(( bids_this_fl + 1 ))
      success "Proposal: FL[$fl_idx] → Job[$job_arr_idx] (${prop_id})"
    else
      err_msg=$(echo "$resp" | jq -r '.error // .message // "unknown"')
      case "$err_msg" in
        *"already submitted"*|*"not accepting"*|*"cannot bid on your own"*|*"only verified"*) ;;
        *) warn "Proposal FL[$fl_idx]→Job[$job_arr_idx]: ${err_msg}" ;;
      esac
    fi
  done
done

info "Total proposals created: ${proposal_count}"

# =============================================================================
# 7. ACCEPT PROPOSALS + CREATE CONTRACTS (target 20)
# =============================================================================
section "7 · Accept proposals & create contracts"

CONTRACT_IDS=()
CONTRACT_CLIENT_IDX=()
CONTRACT_FL_IDX=()
contracts_created=0
TARGET_CONTRACTS=20
declare -A contracted_jobs=()

for prop_arr_idx in "${!PROPOSAL_IDS[@]}"; do
  [[ $contracts_created -ge $TARGET_CONTRACTS ]] && break

  prop_id="${PROPOSAL_IDS[$prop_arr_idx]}"
  job_arr_idx="${PROPOSAL_JOB_IDX[$prop_arr_idx]}"
  fl_idx="${PROPOSAL_FL_IDX[$prop_arr_idx]}"
  [[ -z "$prop_id" || -z "$job_arr_idx" ]] && continue
  [[ -n "${contracted_jobs[$job_arr_idx]+_}" ]] && continue

  job_id="${JOB_IDS[$job_arr_idx]}"
  client_idx="${JOB_CLIENT_IDX[$job_arr_idx]}"
  [[ -z "$job_id" || -z "$client_idx" ]] && continue
  client_token="${CLIENT_TOKENS[$client_idx]}"
  [[ -z "$client_token" ]] && continue

  # ── Accept proposal ──────────────────────────────────────────────────────
  accept_resp=$(post "/api/proposals/${prop_id}/decision" '{"status":"accepted"}' "$client_token")
  accept_status=$(echo "$accept_resp" | jq -r '.status // empty')
  if [[ "$accept_status" != "accepted" ]]; then
    warn "Accept proposal ${prop_id} failed: $(echo "$accept_resp" | jq -r '.error // .message // "unknown"')"
    continue
  fi
  success "Accepted proposal ${prop_id}"

  # ── Build contract JSON ──────────────────────────────────────────────────
  job_budget="${JOB_BUDGETS[$job_arr_idx]:-1000}"
  job_title="${JOB_TITLES[$job_arr_idx]:-Contract}"

  # Portable date arithmetic (GNU date on Linux, BSD date on macOS)
  days_start=$(rand_int 1 7)
  days_end=$(( days_start + $(rand_int 14 30) ))
  start_date=$(date -u -d "+${days_start} days" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null \
    || date -u -v "+${days_start}d" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null \
    || printf '%sT09:00:00Z' "$(date -u '+%Y-%m-%d')")
  end_date=$(date -u -d "+${days_end} days" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null \
    || date -u -v "+${days_end}d" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null \
    || printf '%sT09:00:00Z' "$(date -u '+%Y-%m-%d')")

  contract_body=$(jq -n \
    --arg pid   "$prop_id" \
    --arg title "$job_title" \
    --arg desc  "Full-service delivery of: ${job_title}. Payment is milestone-based with 50% upfront and 50% on completion. All deliverables as specified in the original job post." \
    --argjson   amount "$job_budget" \
    --arg start "$start_date" \
    --arg end   "$end_date" \
    --arg terms "1. All work is original. 2. Deliverables as described in the job post. 3. Up to 3 revision rounds. 4. Payment via Tasksy escrow only. 5. Disputes resolved per Tasksy policy." \
    '{proposal_id:$pid,title:$title,description:$desc,total_amount:$amount,start_date:$start,end_date:$end,terms:$terms}')

  contract_resp=$(post "/api/proposals/create-contract" "$contract_body" "$client_token")
  contract_id=$(echo "$contract_resp" | jq -r '.contract.id // .data.id // empty')

  if [[ -n "$contract_id" ]]; then
    CONTRACT_IDS+=("$contract_id")
    CONTRACT_CLIENT_IDX+=("$client_idx")
    CONTRACT_FL_IDX+=("$fl_idx")
    contracted_jobs["$job_arr_idx"]=1
    contracts_created=$(( contracts_created + 1 ))
    success "Contract ${contracts_created}: \"${job_title:0:42}...\" (${contract_id})"

    # Activate most contracts so disputes can be filed (server requires "active" status)
    if [[ $(( contracts_created % 4 )) -ne 0 ]]; then
      put "/api/v1/admin/contracts/${contract_id}" '{"status":"active"}' "$ADMIN_TOKEN" >/dev/null 2>&1 || true
    fi
  else
    warn "Contract creation failed for proposal ${prop_id}: $(echo "$contract_resp" | jq -r '.error // .message // "unknown"')"
  fi
done

info "Total contracts created: ${contracts_created}"

# =============================================================================
# 8. DISPUTES (~8, filed against active contracts)
# =============================================================================
section "8 · Disputes"

DISPUTE_REASONS=(
  "Work quality below agreed standard"
  "Missed deadline without communication"
  "Deliverables incomplete at final submission"
  "Scope creep — contractor demanding extra payment"
  "Non-responsive for over a week"
  "Final delivery does not match approved mockups"
  "Code contains critical bugs blocking launch"
  "Contractor requesting payment before agreed milestone"
)

DISPUTE_DESCS=(
  "The delivered work does not meet the quality standards we discussed. Specifically, the mobile responsiveness is broken on iOS devices and there are numerous UI inconsistencies. I have sent multiple messages requesting fixes but have received no response in over 5 days. I am requesting fixes within 48 hours or a partial refund."
  "The agreed deadline was 2 weeks from contract start. We are now 6 days past that date with no communication or ETA. The submitted work is approximately 60% complete. This is blocking our product launch and needs urgent resolution."
  "At the point of final delivery, the following items were missing from the agreed scope: the admin dashboard, the CSV export feature, and the email notification system. These were explicitly listed in the job description and discussed during kick-off. I cannot sign off until these are delivered."
  "Midway through the project the contractor informed me they require an additional £500 to complete the original scope. This was not agreed upon and the additional work claimed is already covered under the original deliverables. I am requesting admin mediation."
  "I have sent 11 messages over the past 9 days without a single reply. The last update was incomplete progress notes. I am unable to assess the state of the project. If there is no response within 24 hours I will consider this contract abandoned."
  "The final delivered files are substantially different from the designs I approved two weeks ago. Key brand colours have changed, logo placement is wrong, and typography is inconsistent with our brand guidelines. The contractor appears to have used an older version of the files."
  "The delivered codebase contains critical bugs: authentication fails on Firefox, database queries cause timeouts, and there are security vulnerabilities. The contractor insists the work is complete and refuses to fix these without additional payment."
  "The contractor has requested 75% payment upfront before beginning any work, contradicting our agreed milestone-based schedule of 25% on start, 50% at midpoint, and 25% on final acceptance. I am not comfortable with this deviation."
)

disputes_filed=0
TARGET_DISPUTES=8

for contract_arr_idx in "${!CONTRACT_IDS[@]}"; do
  [[ $disputes_filed -ge $TARGET_DISPUTES ]] && break

  contract_id="${CONTRACT_IDS[$contract_arr_idx]}"
  [[ -z "$contract_id" ]] && continue

  client_idx="${CONTRACT_CLIENT_IDX[$contract_arr_idx]}"
  fl_idx="${CONTRACT_FL_IDX[$contract_arr_idx]}"

  # Alternate filer: even=client, odd=freelancer
  if [[ $(( disputes_filed % 2 )) -eq 0 ]]; then
    filer_token="${CLIENT_TOKENS[$client_idx]}"
    filer_label="Client[$client_idx]"
  else
    filer_token="${FREELANCER_TOKENS[$fl_idx]}"
    filer_label="Freelancer[$fl_idx]"
  fi
  [[ -z "$filer_token" ]] && continue

  ri=$(( disputes_filed % ${#DISPUTE_REASONS[@]} ))
  reason="${DISPUTE_REASONS[$ri]}"
  desc="${DISPUTE_DESCS[$ri]}"

  dispute_body=$(jq -n \
    --arg cid    "$contract_id" \
    --arg reason "$reason" \
    --arg desc   "$desc" \
    '{contract_id:$cid,reason:$reason,description:$desc}')

  resp=$(post "/api/v1/disputes" "$dispute_body" "$filer_token")
  dispute_id=$(echo "$resp" | jq -r '.data.id // empty')

  if [[ -n "$dispute_id" ]]; then
    disputes_filed=$(( disputes_filed + 1 ))
    success "Dispute ${disputes_filed}: ${filer_label} on contract ${contract_id:0:8}... → \"${reason}\""

    # Resolve every other dispute
    if [[ $(( disputes_filed % 2 )) -eq 0 ]]; then
      resolution="After reviewing all communications and evidence submitted by both parties, we have determined that the issue relates to: ${reason,,}. The contractor is required to complete the outstanding items within 5 business days. If not completed, a partial refund of 30% will be issued to the client. Both parties must acknowledge this resolution within 48 hours."
      resolve_body=$(jq -n --arg r "$resolution" '{resolution:$r}')
      res_resp=$(put "/api/v1/admin/disputes/${dispute_id}/resolve" "$resolve_body" "$ADMIN_TOKEN")
      res_status=$(echo "$res_resp" | jq -r '.data.status // empty')
      if [[ "$res_status" == "resolved" ]]; then
        success "  └─ Resolved dispute ${dispute_id:0:8}..."
      else
        warn "  └─ Resolve failed: $(echo "$res_resp" | jq -r '.error // .message // "unknown"')"
      fi
    fi
  else
    err_msg=$(echo "$resp" | jq -r '.error // .message // "unknown"')
    warn "Dispute failed for contract ${contract_id:0:8}...: ${err_msg}"
  fi
done

info "Total disputes filed: ${disputes_filed}"

# =============================================================================
# SUMMARY
# =============================================================================
section "Seed complete"

valid_clients=$(printf '%s\n' "${CLIENT_TOKENS[@]}" | grep -c . || true)
valid_fls=$(printf '%s\n' "${FREELANCER_TOKENS[@]}" | grep -c . || true)
valid_jobs=$(printf '%s\n' "${JOB_IDS[@]}" | grep -c . || true)

echo -e "
  ${GREEN}✔${RESET}  Admin user     : ${ADMIN_EMAIL}  /  ${ADMIN_PASS}
  ${GREEN}✔${RESET}  Clients        : ${valid_clients}  (client.<name>@seed.tasksy.dev  /  ${SEED_PASS})
  ${GREEN}✔${RESET}  Freelancers    : ${valid_fls}  (freelancer.<name>@seed.tasksy.dev  /  ${SEED_PASS})
  ${GREEN}✔${RESET}  Categories     : ${#VALID_CATS[@]}
  ${GREEN}✔${RESET}  Job posts      : ${valid_jobs}
  ${GREEN}✔${RESET}  Proposals      : ${proposal_count}
  ${GREEN}✔${RESET}  Contracts      : ${contracts_created}
  ${GREEN}✔${RESET}  Disputes       : ${disputes_filed}  (~half open, ~half resolved)

  ${CYAN}API${RESET}       : ${BASE_URL}
  ${CYAN}Admin panel${RESET}: http://localhost:3000
"
