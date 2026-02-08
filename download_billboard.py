#!/usr/bin/env python3
"""Download Billboard Hot 100 Year-End Charts (2015-2024)"""

import asyncio
import json
import sys
import time
from pathlib import Path

# Add parent to path for imports
sys.path.insert(0, str(Path(__file__).parent))

from tidal_tui import TidalAPI, DownloadManager, Track

# Billboard Hot 100 Year-End Charts (Top 50 songs per year for 10 years = ~500 songs)
BILLBOARD_CHARTS = {
    2024: [
        ("Shaboozey", "A Bar Song (Tipsy)"),
        ("Teddy Swims", "Lose Control"),
        ("Benson Boone", "Beautiful Things"),
        ("Post Malone ft. Morgan Wallen", "I Had Some Help"),
        ("Sabrina Carpenter", "Espresso"),
        ("Tommy Richman", "Million Dollar Baby"),
        ("Hozier", "Too Sweet"),
        ("Zach Bryan ft. Kacey Musgraves", "I Remember Everything"),
        ("Dua Lipa", "Houdini"),
        ("Sabrina Carpenter", "Please Please Please"),
        ("Jack Harlow", "Lovin On Me"),
        ("Kendrick Lamar", "Not Like Us"),
        ("Doja Cat", "Agora Hills"),
        ("Tyla", "Water"),
        ("SZA", "Saturn"),
        ("Chappell Roan", "Good Luck Babe"),
        ("Billie Eilish", "Birds of a Feather"),
        ("Taylor Swift ft. Post Malone", "Fortnight"),
        ("Ariana Grande", "We Can't Be Friends"),
        ("Beyonce", "Texas Hold Em"),
        ("Future Kendrick Lamar Metro Boomin", "Like That"),
        ("Gunna", "Fukumean"),
        ("Morgan Wallen", "Last Night"),
        ("Tate McRae", "Greedy"),
        ("Noah Kahan", "Stick Season"),
        ("Dasha", "Austin"),
        ("Lady Gaga Bruno Mars", "Die With A Smile"),
        ("Chappell Roan", "HOT TO GO!"),
        ("Eminem", "Houdini"),
        ("Kenya Grace", "Strangers"),
        ("Artemas", "I Like The Way You Kiss Me"),
        ("Sabrina Carpenter", "Taste"),
        ("Jung Kook", "Standing Next to You"),
        ("Sophie Ellis-Bextor", "Murder On The Dancefloor"),
        ("FloyyMenor", "Gata Only"),
        ("Travis Scott", "FE!N"),
        ("Peso Pluma", "La Bebe"),
        ("Jessie Murph", "Wild Ones"),
        ("Megan Thee Stallion", "Mamushi"),
        ("Charli XCX", "Apple"),
        ("SZA", "Snooze"),
        ("21 Savage", "Redrum"),
        ("Xavi", "La Diabla"),
        ("Chris Brown", "Sensational"),
        ("The Weeknd", "Dancing In The Flames"),
        ("Wiz Khalifa", "See You Again"),
        ("SZA", "Kill Bill"),
        ("Ariana Grande", "The Boy Is Mine"),
        ("GloRilla", "Yeah Glo"),
        ("Billie Eilish", "Lunch"),
    ],
    2023: [
        ("Morgan Wallen", "Last Night"),
        ("Miley Cyrus", "Flowers"),
        ("SZA", "Kill Bill"),
        ("Jung Kook ft. Latto", "Seven"),
        ("Taylor Swift", "Anti-Hero"),
        ("Metro Boomin ft. The Weeknd 21 Savage", "Creepin"),
        ("Rema Selena Gomez", "Calm Down"),
        ("Ice Spice", "Boy's a Liar Pt. 2"),
        ("Luke Combs", "Fast Car"),
        ("Doja Cat", "Paint The Town Red"),
        ("David Guetta Bebe Rexha", "I'm Good"),
        ("Tate McRae", "Greedy"),
        ("SZA", "Snooze"),
        ("Jason Aldean", "Try That In A Small Town"),
        ("Zach Bryan", "Something in the Orange"),
        ("Oliver Anthony Music", "Rich Men North Of Richmond"),
        ("Miguel", "Sure Thing"),
        ("Lil Durk", "All My Life"),
        ("Dua Lipa", "Dance The Night"),
        ("Olivia Rodrigo", "Vampire"),
        ("Post Malone", "Chemical"),
        ("Coi Leray", "Players"),
        ("Nicki Minaj Ice Spice", "Barbie World"),
        ("The Weeknd Ariana Grande", "Die For You"),
        ("Teddy Swims", "Lose Control"),
        ("Jelly Roll", "Save Me"),
        ("Ed Sheeran", "Eyes Closed"),
        ("Kenya Grace", "Strangers"),
        ("SZA", "Shirt"),
        ("Bailey Zimmerman", "Rock and A Hard Place"),
        ("Gunna", "Fukumean"),
        ("Peso Pluma Bizarrap", "Quevedo Bzrp Music Sessions 52"),
        ("Jung Kook", "3D"),
        ("Drake 21 Savage", "Rich Flex"),
        ("Noah Kahan", "Stick Season"),
        ("NewJeans", "Super Shy"),
        ("Olivia Rodrigo", "Get Him Back"),
        ("Jack Harlow", "Lovin On Me"),
        ("Travis Scott", "K-POP"),
        ("Nicki Minaj", "Super Freaky Girl"),
        ("Beyonce", "Cuff It"),
        ("Peso Pluma", "Ella Baila Sola"),
        ("Olivia Rodrigo", "Bad Idea Right"),
        ("Rihanna", "Lift Me Up"),
        ("Usher", "Good Good"),
        ("Morgan Wallen", "Thinkin Bout Me"),
        ("Sam Smith Kim Petras", "Unholy"),
        ("Future Metro Boomin", "Type Shit"),
        ("Zach Bryan ft. Kacey Musgraves", "I Remember Everything"),
        ("21 Savage", "Redrum"),
    ],
    2022: [
        ("Harry Styles", "As It Was"),
        ("Glass Animals", "Heat Waves"),
        ("Jack Harlow", "First Class"),
        ("Future ft. Drake Tems", "Wait For U"),
        ("Kate Bush", "Running Up That Hill"),
        ("Steve Lacy", "Bad Habit"),
        ("Harry Styles", "Late Night Talking"),
        ("The Kid Laroi Justin Bieber", "Stay"),
        ("Lizzo", "About Damn Time"),
        ("Latto", "Big Energy"),
        ("Bad Bunny Chencho Corleone", "Me Porto Bonito"),
        ("Post Malone ft. Doja Cat", "I Like You"),
        ("Nicki Minaj", "Super Freaky Girl"),
        ("Bad Bunny", "Titi Me Pregunto"),
        ("Em Beihold", "Numb Little Bug"),
        ("Gunna", "Pushin P"),
        ("Morgan Wallen", "Wasted On You"),
        ("Kodak Black", "Super Gremlin"),
        ("Doja Cat", "Woman"),
        ("Luke Combs", "Doin This"),
        ("Gayle", "ABCDEFU"),
        ("Morgan Wallen", "You Proof"),
        ("Dove Cameron", "Boyfriend"),
        ("Beyonce", "Break My Soul"),
        ("Kendrick Lamar", "N95"),
        ("Sam Smith Kim Petras", "Unholy"),
        ("Bad Bunny", "Moscow Mule"),
        ("OneRepublic", "I Ain't Worried"),
        ("Chris Brown", "Under The Influence"),
        ("Joji", "Glimpse of Us"),
        ("Imagine Dragons JID", "Enemy"),
        ("Anitta", "Envolver"),
        ("Dua Lipa", "Sweetest Pie"),
        ("Silk Sonic", "Smokin Out The Window"),
        ("Charlie Puth", "Light Switch"),
        ("Lil Baby Gunna", "Drip Too Hard"),
        ("We The Kingdom", "Holy Water"),
        ("Nicky Youre", "Sunroof"),
        ("Walker Hayes", "Fancy Like"),
        ("Zach Bryan", "Something in the Orange"),
        ("Bailey Zimmerman", "Fall In Love"),
        ("Drake 21 Savage", "Jimmy Cooks"),
        ("Megan Thee Stallion", "Sweetest Pie"),
        ("Yung Gravy", "Betty"),
        ("Camila Cabello", "Bam Bam"),
        ("Ed Sheeran", "Shivers"),
        ("Elton John Dua Lipa", "Cold Heart"),
        ("Adele", "Easy On Me"),
        ("Silk Sonic", "Love's Train"),
        ("Ghost", "Mary On A Cross"),
    ],
    2021: [
        ("Dua Lipa DaBaby", "Levitating"),
        ("Olivia Rodrigo", "Drivers License"),
        ("The Weeknd", "Blinding Lights"),
        ("Olivia Rodrigo", "Good 4 U"),
        ("Doja Cat SZA", "Kiss Me More"),
        ("Lil Nas X Jack Harlow", "Industry Baby"),
        ("Ed Sheeran", "Bad Habits"),
        ("The Kid Laroi Justin Bieber", "Stay"),
        ("BTS", "Butter"),
        ("Masked Wolf", "Astronaut in the Ocean"),
        ("Bruno Mars Anderson Paak", "Leave The Door Open"),
        ("Ariana Grande", "Positions"),
        ("Glass Animals", "Heat Waves"),
        ("24kGoldn ft. iann dior", "Mood"),
        ("Walker Hayes", "Fancy Like"),
        ("Olivia Rodrigo", "Deja Vu"),
        ("Justin Bieber ft. Daniel Caesar Giveon", "Peaches"),
        ("Polo G", "Rapstar"),
        ("Cardi B", "Up"),
        ("Drake ft. Lil Baby", "Wants And Needs"),
        ("Lil Nas X", "Montero"),
        ("Chris Stapleton", "Starting Over"),
        ("Pop Smoke", "What You Know Bout Love"),
        ("Chris Brown Young Thug", "Go Crazy"),
        ("Giveon", "Heartbreak Anniversary"),
        ("BTS", "Dynamite"),
        ("The Weeknd Ariana Grande", "Save Your Tears"),
        ("Doja Cat", "Need to Know"),
        ("Olivia Rodrigo", "Brutal"),
        ("Morgan Wallen", "Wasted On You"),
        ("Lil Tjay 6lack", "Calling My Phone"),
        ("Jack Harlow", "What's Poppin"),
        ("Dua Lipa", "Don't Start Now"),
        ("Luke Combs", "Forever After All"),
        ("Megan Thee Stallion Beyonce", "Savage"),
        ("Billie Eilish", "Happier Than Ever"),
        ("Drake", "What's Next"),
        ("Ariana Grande The Weeknd", "Off The Table"),
        ("Maroon 5 Megan Thee Stallion", "Beautiful Mistakes"),
        ("Saweetie", "Best Friend"),
        ("Morgan Wallen", "Sand In My Boots"),
        ("Cardi B Megan Thee Stallion", "WAP"),
        ("Moneybagg Yo", "Wockesha"),
        ("Travis Scott", "Franchise"),
        ("Pooh Shiesty", "Back In Blood"),
        ("Harry Styles", "Watermelon Sugar"),
        ("Rod Wave", "Tombstone"),
        ("Farruko", "Pepas"),
        ("Tiesto Ava Max", "The Motto"),
        ("Adele", "Easy On Me"),
    ],
    2020: [
        ("The Weeknd", "Blinding Lights"),
        ("Roddy Ricch", "The Box"),
        ("DaBaby ft. Roddy Ricch", "Rockstar"),
        ("Dua Lipa", "Don't Start Now"),
        ("Harry Styles", "Adore You"),
        ("Post Malone", "Circles"),
        ("Megan Thee Stallion Beyonce", "Savage"),
        ("Doja Cat", "Say So"),
        ("Lewis Capaldi", "Someone You Loved"),
        ("Future ft. Drake", "Life Is Good"),
        ("Harry Styles", "Watermelon Sugar"),
        ("The Scotts", "The Scotts"),
        ("Jawsh 685 Jason Derulo", "Savage Love"),
        ("Jack Harlow", "What's Poppin"),
        ("Chris Brown Young Thug", "Go Crazy"),
        ("SAINt JHN", "Roses"),
        ("Maroon 5", "Memories"),
        ("Justin Bieber ft. Quavo", "Intentions"),
        ("24kGoldn ft. iann dior", "Mood"),
        ("Arizona Zervas", "Roxanne"),
        ("Billie Eilish", "Bad Guy"),
        ("Cardi B Megan Thee Stallion", "WAP"),
        ("Doja Cat Nicki Minaj", "Say So"),
        ("Drake", "Toosie Slide"),
        ("Pop Smoke", "For The Night"),
        ("Lil Baby", "Sum 2 Prove"),
        ("Lil Mosey", "Blueberry Faygo"),
        ("Trevor Daniel", "Falling"),
        ("YoungBoy Never Broke Again", "Bandit"),
        ("BTS", "Dynamite"),
        ("Gabby Barrett", "I Hope"),
        ("Tones And I", "Dance Monkey"),
        ("Lil Uzi Vert", "XO Tour Llif3"),
        ("Pop Smoke", "What You Know Bout Love"),
        ("Black Eyed Peas J Balvin", "Ritmo"),
        ("Drake", "Laugh Now Cry Later"),
        ("Morgan Wallen", "Chasin You"),
        ("Internet Money ft. Gunna", "Lemonade"),
        ("Bad Bunny", "Dakiti"),
        ("Ariana Grande", "Positions"),
        ("Lil Baby Lil Durk", "The Voice"),
        ("Megan Thee Stallion", "Body"),
        ("Luke Combs", "Beautiful Crazy"),
        ("Dan Shay Justin Bieber", "10000 Hours"),
        ("Blake Shelton Gwen Stefani", "Nobody But You"),
        ("Lady Gaga Ariana Grande", "Rain On Me"),
        ("Travis Scott", "Highest In The Room"),
        ("Halsey", "Without Me"),
        ("Blackpink Selena Gomez", "Ice Cream"),
        ("Polo G", "The Goat"),
    ],
    2019: [
        ("Lil Nas X ft. Billy Ray Cyrus", "Old Town Road"),
        ("Post Malone Swae Lee", "Sunflower"),
        ("Ariana Grande", "7 Rings"),
        ("Billie Eilish", "Bad Guy"),
        ("Khalid", "Talk"),
        ("Halsey", "Without Me"),
        ("Post Malone", "Wow"),
        ("Ed Sheeran ft. Justin Bieber", "I Don't Care"),
        ("Panic! At The Disco", "High Hopes"),
        ("Jonas Brothers", "Sucker"),
        ("Lewis Capaldi", "Someone You Loved"),
        ("Shawn Mendes Camila Cabello", "Senorita"),
        ("Travis Scott", "Sicko Mode"),
        ("Lil Tecca", "Ransom"),
        ("Ava Max", "Sweet but Psycho"),
        ("Ariana Grande", "Thank U Next"),
        ("DaBaby", "Suge"),
        ("Lizzo", "Truth Hurts"),
        ("Post Malone", "Circles"),
        ("Chris Brown Drake", "No Guidance"),
        ("Blanco Brown", "The Git Up"),
        ("Sam Smith Normani", "Dancing With A Stranger"),
        ("Drake", "Money In The Grave"),
        ("Juice WRLD", "Lucid Dreams"),
        ("Cardi B Bruno Mars", "Please Me"),
        ("Marshmello ft. Bastille", "Happier"),
        ("Lil Nas X", "Panini"),
        ("Lady Gaga Bradley Cooper", "Shallow"),
        ("Meek Mill Drake", "Going Bad"),
        ("Ariana Grande", "Break Up With Your Girlfriend"),
        ("Y2K bbno$", "Lalala"),
        ("Maroon 5 Cardi B", "Girls Like You"),
        ("Gunna Lil Baby", "Drip Too Hard"),
        ("J Cole", "Middle Child"),
        ("Mustard Roddy Ricch", "Ballin"),
        ("Selena Gomez", "Lose You To Love Me"),
        ("Taylor Swift", "ME!"),
        ("Lizzo", "Good As Hell"),
        ("Young Thug", "The London"),
        ("21 Savage J Cole", "a lot"),
        ("5 Seconds Of Summer", "Youngblood"),
        ("Dan Shay Justin Bieber", "10000 Hours"),
        ("The Weeknd", "Heartless"),
        ("YNW Melly", "Murder On My Mind"),
        ("Tyga", "Taste"),
        ("Luke Combs", "Beautiful Crazy"),
        ("Daddy Yankee ft. Katy Perry", "Con Calma"),
        ("Thomas Rhett", "Look What God Gave Her"),
        ("Maren Morris", "Girl"),
        ("Blake Shelton", "God's Country"),
    ],
    2018: [
        ("Drake", "God's Plan"),
        ("Drake", "Nice For What"),
        ("Cardi B Bad Bunny J Balvin", "I Like It"),
        ("Maroon 5 Cardi B", "Girls Like You"),
        ("Post Malone", "Rockstar"),
        ("XXXTentacion", "SAD!"),
        ("Camila Cabello ft. Young Thug", "Havana"),
        ("Post Malone ft. Ty Dolla Sign", "Psycho"),
        ("Drake", "In My Feelings"),
        ("Juice WRLD", "Lucid Dreams"),
        ("Bebe Rexha Florida Georgia Line", "Meant to Be"),
        ("Ed Sheeran", "Perfect"),
        ("Ella Mai", "Boo'd Up"),
        ("Bruno Mars Cardi B", "Finesse"),
        ("Tyga", "Taste"),
        ("Imagine Dragons", "Thunder"),
        ("6ix9ine ft. Nicki Minaj", "FEFE"),
        ("Cardi B", "Bodak Yellow"),
        ("Travis Scott", "Sicko Mode"),
        ("Dua Lipa", "New Rules"),
        ("5 Seconds Of Summer", "Youngblood"),
        ("Marshmello ft. Bastille", "Happier"),
        ("Khalid Normani", "Love Lies"),
        ("Post Malone", "Better Now"),
        ("Lil Baby Gunna", "Drip Too Hard"),
        ("BTS", "Fake Love"),
        ("Childish Gambino", "This Is America"),
        ("XXXTentacion", "Moonlight"),
        ("Logic ft. Alessia Cara Khalid", "1-800-273-8255"),
        ("Jason Derulo David Guetta", "Goodbye"),
        ("Kendrick Lamar SZA", "All The Stars"),
        ("Calvin Harris Dua Lipa", "One Kiss"),
        ("Shawn Mendes", "In My Blood"),
        ("NF", "Let You Down"),
        ("Zedd Maren Morris Grey", "The Middle"),
        ("Luis Fonsi Demi Lovato", "Echame La Culpa"),
        ("Halsey", "Without Me"),
        ("Kane Brown", "Heaven"),
        ("Lil Pump", "Gucci Gang"),
        ("Migos Drake", "Walk It Talk It"),
        ("G-Eazy Halsey", "Him & I"),
        ("YoungBoy Never Broke Again", "Outside Today"),
        ("Lil Uzi Vert", "XO Tour Llif3"),
        ("Dan Shay", "Tequila"),
        ("Bazzi", "Mine"),
        ("Lauv", "I Like Me Better"),
        ("Ariana Grande", "No Tears Left To Cry"),
        ("Migos", "Stir Fry"),
        ("Post Malone Swae Lee", "Sunflower"),
        ("Florida Georgia Line", "Simple"),
    ],
    2017: [
        ("Ed Sheeran", "Shape of You"),
        ("Luis Fonsi Daddy Yankee ft. Justin Bieber", "Despacito"),
        ("Bruno Mars", "That's What I Like"),
        ("Kendrick Lamar", "Humble"),
        ("Post Malone ft. 21 Savage", "Rockstar"),
        ("The Chainsmokers Coldplay", "Something Just Like This"),
        ("Imagine Dragons", "Believer"),
        ("Lil Uzi Vert", "XO Tour Llif3"),
        ("Cardi B", "Bodak Yellow"),
        ("DJ Khaled ft. Rihanna Bryson Tiller", "Wild Thoughts"),
        ("Future", "Mask Off"),
        ("Sam Hunt", "Body Like A Back Road"),
        ("Migos", "Bad and Boujee"),
        ("French Montana ft. Swae Lee", "Unforgettable"),
        ("The Chainsmokers", "Paris"),
        ("Logic ft. Alessia Cara Khalid", "1-800-273-8255"),
        ("Halsey", "Now or Never"),
        ("Chris Stapleton", "Broken Halos"),
        ("Dua Lipa", "New Rules"),
        ("Camila Cabello ft. Young Thug", "Havana"),
        ("Portugal. The Man", "Feel It Still"),
        ("Kodak Black", "Tunnel Vision"),
        ("Taylor Swift", "Look What You Made Me Do"),
        ("Shawn Mendes", "There's Nothing Holdin' Me Back"),
        ("Imagine Dragons", "Thunder"),
        ("Khalid", "Young Dumb & Broke"),
        ("21 Savage Offset Metro Boomin", "Ric Flair Drip"),
        ("Charlie Puth", "Attention"),
        ("Katy Perry", "Chained To The Rhythm"),
        ("Clean Bandit ft. Sean Paul Anne-Marie", "Rockabye"),
        ("Big Sean", "Bounce Back"),
        ("Maroon 5 SZA", "What Lovers Do"),
        ("DJ Khaled ft. Justin Bieber Quavo", "I'm the One"),
        ("Rihanna", "Love On The Brain"),
        ("Justin Bieber", "Despacito"),
        ("James Arthur", "Say You Won't Let Go"),
        ("ZAYN Taylor Swift", "I Don't Wanna Live Forever"),
        ("Zay Hilfigerrr Zayion McCall", "Juju On That Beat"),
        ("Kyle ft. Lil Yachty", "iSpy"),
        ("Machine Gun Kelly Camila Cabello", "Bad Things"),
        ("Gucci Mane Migos", "Slippery"),
        ("Alessia Cara", "Scars To Your Beautiful"),
        ("Rae Sremmurd", "Black Beatles"),
        ("G-Eazy ft. A$AP Rocky Cardi B", "No Limit"),
        ("Cardi B", "Bartier Cardi"),
        ("Niall Horan", "Slow Hands"),
        ("Lil Pump", "Gucci Gang"),
        ("Selena Gomez", "Bad Liar"),
        ("Post Malone", "Congratulations"),
        ("Ed Sheeran", "Castle On The Hill"),
    ],
    2016: [
        ("Justin Timberlake", "Can't Stop The Feeling"),
        ("Rihanna ft. Drake", "Work"),
        ("Drake", "One Dance"),
        ("The Chainsmokers ft. Halsey", "Closer"),
        ("Twenty One Pilots", "Stressed Out"),
        ("Desiigner", "Panda"),
        ("Sia", "Cheap Thrills"),
        ("Drake ft. WizKid Kyla", "One Dance"),
        ("Lukas Graham", "7 Years"),
        ("Adele", "Hello"),
        ("D.R.A.M. ft. Lil Yachty", "Broccoli"),
        ("Drake ft. Rihanna", "Too Good"),
        ("Rae Sremmurd", "Black Beatles"),
        ("The Chainsmokers ft. Daya", "Don't Let Me Down"),
        ("Fifth Harmony ft. Ty Dolla Sign", "Work From Home"),
        ("Bryson Tiller", "Exchange"),
        ("DJ Snake ft. Justin Bieber", "Let Me Love You"),
        ("Meghan Trainor", "No"),
        ("Calvin Harris ft. Rihanna", "This Is What You Came For"),
        ("Mike Posner", "I Took A Pill In Ibiza"),
        ("Rihanna", "Needed Me"),
        ("Twenty One Pilots", "Ride"),
        ("Shawn Mendes", "Treat You Better"),
        ("Zayn", "Pillowtalk"),
        ("Kent Jones", "Don't Mind"),
        ("Flo Rida", "My House"),
        ("Charlie Puth ft. Selena Gomez", "We Don't Talk Anymore"),
        ("G-Eazy Bebe Rexha", "Me Myself & I"),
        ("Ariana Grande", "Into You"),
        ("Justin Bieber", "Love Yourself"),
        ("Ariana Grande", "Dangerous Woman"),
        ("Alessia Cara", "Here"),
        ("Kungs", "This Girl"),
        ("Rihanna Drake", "Work"),
        ("Gnash ft. Olivia O'Brien", "I Hate U I Love U"),
        ("Major Lazer ft. Justin Bieber MO", "Cold Water"),
        ("The Weeknd ft. Daft Punk", "Starboy"),
        ("Meghan Trainor", "Me Too"),
        ("21 Pilots", "Heathens"),
        ("Desiigner", "Timmy Turner"),
        ("Nicky Jam", "Hasta El Amanecer"),
        ("Selena Gomez", "Hands To Myself"),
        ("Jax Jones", "You Don't Know Me"),
        ("Beyonce", "Formation"),
        ("The Weeknd", "Can't Feel My Face"),
        ("Drake", "Hotline Bling"),
        ("Bruno Mars", "24K Magic"),
        ("Yo Gotti", "Down In The DM"),
        ("Beyonce", "Sorry"),
        ("Fetty Wap", "679"),
    ],
    2015: [
        ("Mark Ronson ft. Bruno Mars", "Uptown Funk"),
        ("Ed Sheeran", "Thinking Out Loud"),
        ("Wiz Khalifa ft. Charlie Puth", "See You Again"),
        ("Fetty Wap", "Trap Queen"),
        ("The Weeknd", "The Hills"),
        ("Taylor Swift", "Bad Blood"),
        ("Silento", "Watch Me"),
        ("The Weeknd", "Earned It"),
        ("OMI", "Cheerleader"),
        ("Rachel Platten", "Fight Song"),
        ("Major Lazer DJ Snake ft. MO", "Lean On"),
        ("Walk The Moon", "Shut Up and Dance"),
        ("Taylor Swift", "Blank Space"),
        ("Justin Bieber", "What Do You Mean"),
        ("Maroon 5", "Sugar"),
        ("Drake", "Hotline Bling"),
        ("Ellie Goulding", "Love Me Like You Do"),
        ("The Weeknd", "Can't Feel My Face"),
        ("Hozier", "Take Me To Church"),
        ("Carly Rae Jepsen", "I Really Like You"),
        ("Taylor Swift", "Style"),
        ("Rihanna Kanye West Paul McCartney", "FourFiveSeconds"),
        ("Selena Gomez", "Good For You"),
        ("Adele", "Hello"),
        ("Jason Derulo", "Want To Want Me"),
        ("Sam Smith", "Stay With Me"),
        ("Meghan Trainor", "Lips Are Movin"),
        ("Andy Grammer", "Honey I'm Good"),
        ("Demi Lovato", "Cool For The Summer"),
        ("Chris Brown Tyga", "Ayo"),
        ("Ariana Grande", "One Last Time"),
        ("Nick Jonas", "Jealous"),
        ("Natalie La Rose ft. Jeremih", "Somebody"),
        ("Big Sean", "I Don't F*** With You"),
        ("Charlie Puth Meghan Trainor", "Marvin Gaye"),
        ("Fifth Harmony ft. Kid Ink", "Worth It"),
        ("Ed Sheeran", "Photograph"),
        ("Nate Ruess", "Nothing Without Love"),
        ("Justin Bieber", "Sorry"),
        ("Drake ft. Future", "Where Ya At"),
        ("Shawn Mendes", "Stitches"),
        ("Fall Out Boy", "Uma Thurman"),
        ("Tove Lo", "Talking Body"),
        ("Omarion ft. Chris Brown Jhene Aiko", "Post To Be"),
        ("Pitbull ft. Robin Thicke", "Time Of Our Lives"),
        ("Sam Hunt", "Take Your Time"),
        ("Imagine Dragons", "Shots"),
        ("Rae Sremmurd", "No Type"),
        ("Twenty One Pilots", "Stressed Out"),
        ("Beyonce", "Drunk In Love"),
    ],
}


async def main():
    print("=== DOWNLOADING BILLBOARD HOT 100 (2015-2024) ===\n")
    print("10 years × ~50 songs = ~500 top hits\n")

    api = TidalAPI()
    download_manager = DownloadManager()
    await download_manager.connect()

    all_tracks = []
    total_found = 0
    total_missed = 0

    for year in sorted(BILLBOARD_CHARTS.keys(), reverse=True):
        songs = BILLBOARD_CHARTS[year]
        print(f"\n[{year}] Searching for {len(songs)} songs...")
        year_found = 0

        for i, (artist, title) in enumerate(songs):
            # Search for the track
            query = f"{artist} {title}"
            try:
                results = await api.search_tracks(query, limit=5)

                if results:
                    # Take the first result
                    track = results[0]
                    all_tracks.append(track)
                    year_found += 1
                    if (i + 1) % 10 == 0:
                        print(f"  [{i+1}/{len(songs)}] Found: {track.title} by {track.artist}")
                else:
                    total_missed += 1
                    if total_missed <= 20:  # Only print first 20 misses
                        print(f"  [!] Not found: {artist} - {title}")

            except Exception as e:
                total_missed += 1
                print(f"  [!] Error: {artist} - {title}: {e}")

            # Small delay to avoid rate limiting
            await asyncio.sleep(0.05)

        total_found += year_found
        print(f"  {year}: Found {year_found}/{len(songs)} songs")

    print(f"\n=== SEARCH COMPLETE ===")
    print(f"Total found: {total_found}")
    print(f"Total missed: {total_missed}")
    print(f"Total tracks: {len(all_tracks)}")

    if all_tracks:
        print(f"\nAdding {len(all_tracks)} tracks to queue...")
        added = download_manager.add_tracks(all_tracks)
        print(f"Added: {added} tracks (skipped {len(all_tracks) - added} duplicates/existing)")

        if added > 0:
            print(f"\n=== STARTING DOWNLOAD ({added} tracks) ===")
            download_manager.stats.session_start = time.time()
            download_manager.stats.session_bytes = 0

            # Track progress
            completed = 0
            failed = 0
            start_time = time.time()

            async def progress_callback():
                nonlocal completed, failed
                stats = download_manager.get_stats()
                completed = stats["completed"]
                failed = stats["failed"]
                total = stats["total"]
                speed = stats["speed"]
                bytes_dl = stats["bytes"]

                elapsed = time.time() - start_time
                if elapsed > 0:
                    avg_speed = (bytes_dl / 1024 / 1024) / elapsed

                print(f"Progress: {completed}/{total} | Speed: {avg_speed:.1f} MB/s | Downloaded: {bytes_dl/1024/1024/1024:.2f} GB | Failed: {failed}")

            # Start downloads
            await download_manager.download_all(progress_callback=progress_callback)

            final_stats = download_manager.get_stats()
            elapsed = time.time() - start_time

            print(f"\n=== DOWNLOAD COMPLETE ===")
            print(f"Completed: {final_stats['completed']}")
            print(f"Failed: {final_stats['failed']}")
            print(f"Total size: {final_stats['bytes']/1024/1024/1024:.2f} GB")
            print(f"Total time: {elapsed/60:.1f} minutes")
            print(f"Avg speed: {(final_stats['bytes']/1024/1024)/max(elapsed,1):.1f} MB/s")
        else:
            print("All tracks already downloaded!")
    else:
        print("No tracks found to download.")


if __name__ == "__main__":
    asyncio.run(main())
